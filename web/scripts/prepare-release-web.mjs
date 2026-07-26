import { cpSync, mkdirSync, rmSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const distDir = path.join(root, 'dist')
const releaseDir = path.resolve(root, '..', 'release', 'web')

rmSync(releaseDir, { recursive: true, force: true })
mkdirSync(releaseDir, { recursive: true })
cpSync(distDir, releaseDir, { recursive: true })

console.log(`已生成发布静态资源: ${releaseDir}`)
console.log('请将 release/web/ 目录内容放入与 tj.exe 同级的 web/ 后打包 ZIP')
