import { access, mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const requestedTarget = process.argv.find((argument) => argument.startsWith('--target='))?.split('=')[1]
  ?? (process.argv.includes('--target') ? process.argv[process.argv.indexOf('--target') + 1] : 'mobile')
const target = requestedTarget === 'tv' ? 'tv' : 'mobile'
if (requestedTarget !== target) {
  throw new Error(`Unsupported Android target: ${requestedTarget}`)
}

const overlayResDir = resolve(repoRoot, 'src-tauri/android-res')
const targetResDir = resolve(repoRoot, `src-tauri/android-res-${target}`)
const overlayAndroidMainDir = resolve(
  repoRoot,
  target === 'tv' ? 'src-tauri/android-src/tv-main' : 'src-tauri/android-src/main',
)
const androidMainDir = resolve(repoRoot, 'src-tauri/gen/android/app/src/main')
const androidResDir = resolve(repoRoot, 'src-tauri/gen/android/app/src/main/res')
const androidManifest = resolve(repoRoot, 'src-tauri/gen/android/app/src/main/AndroidManifest.xml')
const androidBuildGradle = resolve(repoRoot, 'src-tauri/gen/android/app/build.gradle.kts')
const androidNamespace = 'vip.baka.bangumipipeline.mobile'
const applicationID =
  target === 'tv' ? 'vip.baka.bangumipipeline.tv' : 'vip.baka.bangumipipeline.mobile'
const staleTVPackageDir = resolve(
  androidMainDir,
  'java/vip/baka/bangumipipeline/tv',
)
const tvOnlyResourcePaths = [
  resolve(androidResDir, 'drawable/tv_banner.xml'),
  resolve(androidResDir, 'drawable/tv_banner_background.xml'),
]

async function copyDirectoryContents(sourceDir, targetDir) {
  await mkdir(targetDir, { recursive: true })

  const entries = await readdir(sourceDir, { withFileTypes: true })
  for (const entry of entries) {
    const sourcePath = join(sourceDir, entry.name)
    const targetPath = join(targetDir, entry.name)

    if (entry.isDirectory()) {
      await copyDirectoryContents(sourcePath, targetPath)
      continue
    }

    if (entry.isFile()) {
      await mkdir(dirname(targetPath), { recursive: true })
      await writeFile(targetPath, await readFile(sourcePath))
    }
  }
}

async function directoryExists(path) {
  try {
    await access(path)
    return true
  } catch {
    return false
  }
}

try {
  await access(androidManifest)
  await access(androidBuildGradle)
} catch {
  console.warn('Android project is not initialized yet. Skipped Android target preparation.')
  process.exit(0)
}

await mkdir(androidResDir, { recursive: true })
await rm(staleTVPackageDir, { recursive: true, force: true })
for (const resourcePath of tvOnlyResourcePaths) {
  await rm(resourcePath, { force: true })
}
await copyDirectoryContents(overlayAndroidMainDir, androidMainDir)
await copyDirectoryContents(overlayResDir, androidResDir)
if (await directoryExists(targetResDir)) {
  await copyDirectoryContents(targetResDir, androidResDir)
}

const buildGradle = await readFile(androidBuildGradle, 'utf8')
const preparedBuildGradle = buildGradle
  .replace(/namespace\s*=\s*"[^"]+"/, `namespace = "${androidNamespace}"`)
  .replace(/applicationId\s*=\s*"[^"]+"/, `applicationId = "${applicationID}"`)
if (
  !preparedBuildGradle.includes(`namespace = "${androidNamespace}"`)
  || !preparedBuildGradle.includes(`applicationId = "${applicationID}"`)
) {
  throw new Error('Unable to update Android namespace and applicationId.')
}
await writeFile(androidBuildGradle, preparedBuildGradle, 'utf8')

console.log(`Prepared Android ${target} target (${applicationID}).`)
