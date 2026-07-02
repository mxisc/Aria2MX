#!/usr/bin/env node

import { mkdtemp, mkdir, rm, writeFile, readFile, readdir } from 'node:fs/promises'
import { createWriteStream } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { Readable } from 'node:stream'
import { pipeline } from 'node:stream/promises'
import { spawnSync } from 'node:child_process'

const projectRoot = process.cwd()
const outputDir = path.join(projectRoot, 'internal', 'aria2embed', 'runtime')

const targets = {
  'darwin-arm64': {
    subdir: 'osx-arm64',
    extraPackages: ['libcxx'],
  },
  'linux-amd64': {
    subdir: 'linux-64',
    extraPackages: [],
  },
  'linux-arm64': {
    subdir: 'linux-aarch64',
    extraPackages: [],
  },
}

const archAliases = {
  x64: 'amd64',
  amd64: 'amd64',
  arm64: 'arm64',
}

function detectDefaultTarget() {
  const goos = process.env.GOOS || process.platform
  const rawArch = process.env.GOARCH || process.arch
  const goarch = archAliases[rawArch]
  const target = `${goos}-${goarch}`
  if (!goarch || !(target in targets)) {
    throw new Error(`Unsupported host target: ${goos}-${rawArch}`)
  }
  return target
}

const requestedTargets = process.argv.slice(2)
const selectedTargets =
  requestedTargets.length === 0
    ? [detectDefaultTarget()]
    : requestedTargets.includes('all')
      ? Object.keys(targets)
      : requestedTargets

for (const target of selectedTargets) {
  if (!(target in targets)) {
    console.error(`Unsupported target: ${target}`)
    process.exit(1)
  }
}

async function prepareTarget(target, config) {
  console.log(`Preparing aria2 runtime for ${target}`)
  const tempRoot = await mkdtemp(path.join(tmpdir(), `aria2mx-${target}-`))
  const downloadDir = path.join(tempRoot, 'downloads')
  const stageDir = path.join(tempRoot, 'stage')
  await mkdir(downloadDir, { recursive: true })
  await mkdir(stageDir, { recursive: true })

  try {
    const resolver = new CondaResolver(config.subdir)
    await resolver.collect('aria2', '1.37.0')
    for (const extra of config.extraPackages) {
      await resolver.collect(extra)
    }

    for (const dist of resolver.resolved.values()) {
      const archivePath = path.join(downloadDir, path.basename(dist.downloadURL))
      await downloadFile(dist.downloadURL, archivePath)
      await extractCondaPackage(archivePath, stageDir)
    }

    await pruneStage(stageDir)

    const manifest = {
      generatedAt: new Date().toISOString(),
      target,
      subdir: config.subdir,
      packages: [...resolver.resolved.values()].map((item) => ({
        name: item.packageName,
        version: item.version,
        basename: item.basename,
      })),
    }
    await writeFile(path.join(stageDir, '.aria2mx-runtime-manifest.json'), JSON.stringify(manifest, null, 2))

    const outputPath = path.join(outputDir, `${target}.tar.gz`)
    const tar = spawnSync('tar', ['-czf', outputPath, '-C', stageDir, '.'], {
      cwd: projectRoot,
      stdio: 'inherit',
    })
    if (tar.status !== 0) {
      throw new Error(`tar failed for ${target}`)
    }

    console.log(`Wrote ${outputPath}`)
  } finally {
    await rm(tempRoot, { recursive: true, force: true })
  }
}

class CondaResolver {
  constructor(subdir) {
    this.subdir = subdir
    this.resolved = new Map()
    this.packageMeta = new Map()
  }

  async collect(packageName, versionHint) {
    if (this.resolved.has(packageName)) {
      return
    }
    const meta = await this.fetchPackageMeta(packageName)
    const dist = this.pickDistribution(meta, versionHint)
    this.resolved.set(packageName, dist)
    for (const dependency of dist.dependencies) {
      if (shouldSkipDependency(dependency)) {
        continue
      }
      await this.collect(dependency)
    }
  }

  async fetchPackageMeta(packageName) {
    if (this.packageMeta.has(packageName)) {
      return this.packageMeta.get(packageName)
    }
    const response = await fetch(`https://api.anaconda.org/package/conda-forge/${packageName}`)
    if (!response.ok) {
      throw new Error(`Failed to fetch metadata for ${packageName}`)
    }
    const meta = await response.json()
    this.packageMeta.set(packageName, meta)
    return meta
  }

  pickDistribution(meta, versionHint) {
    const files = (meta.files || []).filter((file) => {
      const subdir = file.attrs?.subdir || basenameSubdir(file.basename)
      if (subdir !== this.subdir) {
        return false
      }
      if (file.distribution_type !== 'conda') {
        return false
      }
      const version = file.version || file.attrs?.version || parseVersionFromBasename(file.basename)
      return !versionHint || version === versionHint
    })
    if (files.length === 0) {
      throw new Error(`No distribution found for ${meta.name} on ${this.subdir}`)
    }
    files.sort((a, b) => new Date(b.upload_time).getTime() - new Date(a.upload_time).getTime())
    const selected = files[0]
    const version = selected.version || selected.attrs?.version || parseVersionFromBasename(selected.basename)
    return {
      packageName: meta.name,
      version,
      basename: selected.basename,
      downloadURL: `https://anaconda.org/conda-forge/${meta.name}/${version}/download/${selected.basename}`,
      dependencies: parseDependencies(selected),
    }
  }
}

function parseDependencies(file) {
  const raw = file.dependencies?.depends || file.attrs?.depends || []
  return raw
    .map((item) => (typeof item === 'string' ? item.split(' ')[0] : item.name))
    .filter(Boolean)
}

function parseVersionFromBasename(basename = '') {
  const filename = basename.split('/').pop() || basename
  const match = filename.match(/^[^-]+-([^-]+)-/)
  return match ? match[1] : ''
}

function basenameSubdir(basename = '') {
  return basename.split('/')[0] || ''
}

function shouldSkipDependency(name) {
  return name.startsWith('__') || name === 'python_abi' || name === 'ca-certificates' || name === 'tzdata'
}

async function downloadFile(url, targetPath) {
  try {
    await readFile(targetPath)
    return
  } catch {}

  const response = await fetch(url)
  if (!response.ok || !response.body) {
    throw new Error(`Failed to download ${url}`)
  }
  await pipeline(Readable.fromWeb(response.body), createWriteStream(targetPath))
}

async function extractCondaPackage(archivePath, stageDir) {
  const outerDir = await mkdtemp(path.join(tmpdir(), 'aria2mx-conda-'))
  try {
    let result = spawnSync('bsdtar', ['-xf', archivePath, '-C', outerDir], { stdio: 'inherit' })
    if (result.status !== 0) {
      throw new Error(`Failed to unpack ${archivePath}`)
    }
    const files = await readdir(outerDir)
    const pkgArchive = files.find((name) => name.startsWith('pkg-') && name.endsWith('.tar.zst'))
    if (!pkgArchive) {
      throw new Error(`pkg archive missing in ${archivePath}`)
    }
    result = spawnSync('bsdtar', ['-xf', path.join(outerDir, pkgArchive), '-C', stageDir], { stdio: 'inherit' })
    if (result.status !== 0) {
      throw new Error(`Failed to extract payload from ${archivePath}`)
    }
  } finally {
    await rm(outerDir, { recursive: true, force: true })
  }
}

async function pruneStage(stageDir) {
  const removable = [
    'etc/conda',
    'include',
    'info',
    'lib/pkgconfig',
    'lib/cmake',
    'lib/icu',
    'share/doc',
    'share/icu',
    'share/info',
    'share/licenses',
    'share/locale',
    'share/man',
    'share',
  ]
  for (const entry of removable) {
    await rm(path.join(stageDir, entry), { recursive: true, force: true })
  }

  const binDir = path.join(stageDir, 'bin')
  try {
    const entries = await readdir(binDir)
    for (const entry of entries) {
      if (entry !== 'aria2c') {
        await rm(path.join(binDir, entry), { recursive: true, force: true })
      }
    }
  } catch {}

  const libDir = path.join(stageDir, 'lib')
  try {
    const entries = await readdir(libDir)
    for (const entry of entries) {
      if (entry.endsWith('.a')) {
        await rm(path.join(libDir, entry), { force: true })
      }
    }
  } catch {}
}

await mkdir(outputDir, { recursive: true })

for (const target of selectedTargets) {
  await prepareTarget(target, targets[target])
}
