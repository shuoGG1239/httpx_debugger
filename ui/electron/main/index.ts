import { app, BrowserWindow, dialog, ipcMain } from 'electron'
import { spawn, type ChildProcess } from 'node:child_process'
import { existsSync, writeFile } from 'node:fs'
import { join } from 'node:path'
import { CoreClient } from './core-client'

const PROXY_LISTEN = '127.0.0.1:8899'
const CORE_PORT = 8900

let mainWindow: BrowserWindow | null = null
let core: ChildProcess | null = null
let coreClient: CoreClient | null = null
let coreBinary: string | null = null
let coreError: string | null = null

function resolveCoreBinary(): string | null {
  const candidates: string[] = []
  if (process.env.HTTPX_CORE_BIN) candidates.push(process.env.HTTPX_CORE_BIN)
  // dev：ui/ 的上级 core/；打包后：resources/httpx-core.exe
  candidates.push(
    join(app.getAppPath(), '..', 'core', 'httpx-core.exe'),
    join(process.cwd(), '..', 'core', 'httpx-core.exe'),
    join(process.resourcesPath, 'httpx-core.exe'),
  )
  for (const c of candidates) {
    try {
      if (existsSync(c)) return c
    } catch {
      // ignore
    }
  }
  return null
}

function startCore(): void {
  const bin = resolveCoreBinary()
  coreBinary = bin
  if (!bin) {
    coreError = '未找到 httpx-core.exe。构建方法见 README：cd core && go build -o httpx-core.exe .'
    emitLifecycle()
    return
  }
  const dataDir = join(app.getPath('userData'), 'ca')
  core = spawn(bin, ['serve', '--data-dir', dataDir, '--listen', PROXY_LISTEN, '--port', String(CORE_PORT)], {
    windowsHide: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  core.stdout?.on('data', (d) => process.stdout.write(`[core] ${d}`))
  core.stderr?.on('data', (d) => process.stderr.write(`[core] ${d}`))
  core.on('exit', (code, signal) => {
    core = null
    coreError = `httpx-core 已退出 (code=${code} signal=${signal ?? ''})`
    emitLifecycle()
  })
  core.on('error', (err) => {
    coreError = `启动 httpx-core 失败: ${err.message}`
    emitLifecycle()
  })
}

function stopCore(): void {
  // stop 命令会自动还原系统代理（start 时自动接管）
  coreClient?.send({ cmd: 'stop' })
  coreClient?.stop()
  if (core) {
    const c = core
    core = null
    c.kill()
  }
}

function emitLifecycle(): void {
  try {
    mainWindow?.webContents.send('core:lifecycle', {
      connected: coreClient?.isConnected() ?? false,
      binary: coreBinary,
      error: coreError,
    })
  } catch {
    // 窗口已销毁（退出中）时忽略
  }
}

function createWindow(): void {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 960,
    minHeight: 640,
    show: false,
    title: 'HTTPX Debugger',
    backgroundColor: '#111418',
    titleBarStyle: 'hidden',
    titleBarOverlay: {
      color: '#111418',
      symbolColor: '#9aa4b2',
      height: 38,
    },
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
    },
  })

  mainWindow.on('ready-to-show', () => {
    mainWindow?.show()
  })

  mainWindow.webContents.on('before-input-event', (event, input) => {
    if (input.type === 'keyDown' && input.key === 'F12') {
      mainWindow?.webContents.toggleDevTools()
    }
  })

  if (process.env.ELECTRON_RENDERER_URL) {
    mainWindow.loadURL(process.env.ELECTRON_RENDERER_URL)
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

function registerIpc(): void {
  ipcMain.on('core:send', (_e, msg: unknown) => {
    if (process.env.HTTPX_SMOKE) console.log('[smoke] core:send', JSON.stringify(msg))
    coreClient?.send(msg)
  })

  // renderer 挂载后主动拉取当前连接状态（补发在挂载前丢失的 lifecycle 事件）
  ipcMain.handle('core:getLifecycle', () => {
    if (process.env.HTTPX_SMOKE) console.log('[smoke] getLifecycle called')
    return {
      connected: coreClient?.isConnected() ?? false,
      binary: coreBinary,
      error: coreError,
    }
  })

  ipcMain.handle('har:save', async (_e, json: string, defaultName: string) => {
    const win = mainWindow
    if (!win) return { ok: false, error: 'no window' }
    const result = await dialog.showSaveDialog(win, {
      title: '导出 HAR',
      defaultPath: defaultName,
      filters: [{ name: 'HAR 文件', extensions: ['har'] }],
    })
    if (result.canceled || !result.filePath) return { ok: false, canceled: true }
    try {
      await new Promise<void>((resolve, reject) => {
        writeFile(result.filePath!, json, 'utf8', (err) => (err ? reject(err) : resolve()))
      })
      return { ok: true, path: result.filePath }
    } catch (err) {
      return { ok: false, error: err instanceof Error ? err.message : String(err) }
    }
  })
}

app.whenReady().then(() => {
  if (process.platform === 'win32') {
    app.setAppUserModelId('com.httpx.debugger')
  }
  startCore()
  coreClient = new CoreClient(
    CORE_PORT,
    (msg) => {
      if (process.env.HTTPX_SMOKE) {
        const m = msg as { type?: string }
        if (m.type === 'session') {
          const s = (msg as { session?: { method: string; url: string; status: number } }).session
          console.log(`[smoke] session ${s?.method} ${s?.url} -> ${s?.status}`)
        } else if (m.type === 'interceptRequest' || m.type === 'interceptResponse') {
          console.log(`[smoke] ${m.type} ${JSON.stringify(msg).slice(0, 160)}`)
        } else if (m.type === 'status') {
          const st = (msg as { status?: { caInstalled?: boolean; capturing?: boolean } }).status
          console.log(`[smoke] status caInstalled=${st?.caInstalled} capturing=${st?.capturing}`)
        }
      }
      mainWindow?.webContents.send('core:event', msg)
    },
    (state) => {
      coreError = state.error ?? null
      emitLifecycle()
    },
  )
  coreClient.start()
  registerIpc()
  createWindow()

  // HTTPX_SMOKE=1：验证主进程 + core 拉起 + renderer 加载 + IPC 回路后自动退出
  // HTTPX_SMOKE_MS 可覆盖默认 8s 窗口
  if (process.env.HTTPX_SMOKE) {
    const ms = Number(process.env.HTTPX_SMOKE_MS || 8000)
    const timer = setTimeout(() => {
      console.log('SMOKE_OK')
      app.quit()
    }, ms)
    app.on('will-quit', () => clearTimeout(timer))
  }

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      createWindow()
    }
  })
})

app.on('before-quit', () => {
  stopCore()
})

app.on('window-all-closed', () => {
  stopCore()
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
