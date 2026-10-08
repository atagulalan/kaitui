#!/usr/bin/env node
'use strict';

const { spawnSync } = require('child_process');
const fs = require('fs');
const https = require('https');
const path = require('path');
const os = require('os');

const VERSION = require('../package.json').version;
const REPO = 'atagulalan/kaitui';
const cwd = process.cwd();
const localBin = path.join(cwd, process.platform === 'win32' ? 'kaitui.exe' : 'kaitui');
const localKai = path.join(cwd, 'kai');

function die(msg, code = 1) {
  console.error(msg);
  process.exit(code);
}

function platformAsset() {
  const p = process.platform;
  const a = process.arch;
  const map = {
    'linux-x64': 'kaitui-linux-amd64',
    'linux-arm64': 'kaitui-linux-arm64',
    'darwin-x64': 'kaitui-darwin-amd64',
    'darwin-arm64': 'kaitui-darwin-arm64',
    'win32-x64': 'kaitui-windows-amd64.exe',
  };
  const key = `${p}-${a}`;
  const name = map[key];
  if (!name) die(`kaitui: unsupported platform ${key}`);
  return name;
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const follow = (u, redirects = 0) => {
      https
        .get(u, (res) => {
          if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            if (redirects > 5) return reject(new Error('too many redirects'));
            return follow(res.headers.location, redirects + 1);
          }
          if (res.statusCode !== 200) {
            reject(new Error(`download failed: HTTP ${res.statusCode} for ${u}`));
            return;
          }
          const out = fs.createWriteStream(dest, { mode: 0o755 });
          res.pipe(out);
          out.on('finish', () => out.close(resolve));
          out.on('error', reject);
        })
        .on('error', reject);
    };
    follow(url);
  });
}

function ensureKai() {
  try {
    fs.accessSync(localKai, fs.constants.X_OK);
    return;
  } catch {
    /* missing */
  }
  console.error('kaitui: ./kai missing — running npx kaijou…');
  const r = spawnSync(
    'npx',
    ['--yes', 'kaijou'],
    { cwd, stdio: 'inherit', shell: process.platform === 'win32' },
  );
  if (r.status !== 0) {
    die('kaitui: could not install kai. Run: npx kaijou');
  }
}

async function ensureBinary() {
  try {
    fs.accessSync(localBin, fs.constants.X_OK);
    // avoid using a directory named kaitui
    if (!fs.statSync(localBin).isDirectory()) return localBin;
  } catch {
    /* install */
  }

  const asset = platformAsset();
  const url = `https://github.com/${REPO}/releases/download/v${VERSION}/${asset}`;
  const tmp = path.join(os.tmpdir(), asset);
  console.error(`kaitui: downloading ${asset}…`);
  try {
    await download(url, tmp);
  } catch (e) {
    die(
      `kaitui: download failed (${e.message}).\n` +
        `Build manually: go install github.com/${REPO}@v${VERSION}\n` +
        `Or: npx github:${REPO} after a release exists.`,
    );
  }
  fs.copyFileSync(tmp, localBin);
  fs.chmodSync(localBin, 0o755);
  try {
    fs.unlinkSync(tmp);
  } catch {
    /* ignore */
  }
  console.error('kaitui: installed ./kaitui');
  return localBin;
}

async function main() {
  ensureKai();
  const bin = await ensureBinary();
  const args = process.argv.slice(2);
  const r = spawnSync(bin, args, { cwd, stdio: 'inherit' });
  if (r.error) die('kaitui: failed to run: ' + r.error.message);
  process.exit(r.status == null ? 1 : r.status);
}

main().catch((e) => die(String(e && e.stack ? e.stack : e)));
