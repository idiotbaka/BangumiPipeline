import { access, mkdir, readdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const overlayResDir = resolve(repoRoot, 'src-tauri/android-res')
const overlayAndroidMainDir = resolve(repoRoot, 'src-tauri/android-src/main')
const androidMainDir = resolve(repoRoot, 'src-tauri/gen/android/app/src/main')
const androidResDir = resolve(repoRoot, 'src-tauri/gen/android/app/src/main/res')
const androidManifest = resolve(repoRoot, 'src-tauri/gen/android/app/src/main/AndroidManifest.xml')

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

try {
  await access(androidManifest)
} catch {
  console.warn('Android project is not initialized yet. Skipped splash resource preparation.')
  process.exit(0)
}

await mkdir(androidResDir, { recursive: true })
await copyDirectoryContents(overlayAndroidMainDir, androidMainDir)
await copyDirectoryContents(overlayResDir, androidResDir)

console.log('Prepared Android app resources.')
