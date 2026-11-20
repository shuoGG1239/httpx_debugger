import { contextBridge, ipcRenderer, type IpcRendererEvent } from 'electron'

export interface Api {
  sendCore: (msg: unknown) => void
  onCoreEvent: (cb: (msg: unknown) => void) => () => void
  onCoreLifecycle: (cb: (state: unknown) => void) => () => void
  getLifecycle: () => Promise<{ connected: boolean; binary: string | null; error: string | null }>
  saveHar: (json: string, defaultName: string) => Promise<{
    ok: boolean
    canceled?: boolean
    path?: string
    error?: string
  }>
}

const api: Api = {
  sendCore: (msg) => ipcRenderer.send('core:send', msg),
  onCoreEvent: (cb) => {
    const handler = (_e: IpcRendererEvent, msg: unknown) => cb(msg)
    ipcRenderer.on('core:event', handler)
    return () => ipcRenderer.removeListener('core:event', handler)
  },
  onCoreLifecycle: (cb) => {
    const handler = (_e: IpcRendererEvent, state: unknown) => cb(state)
    ipcRenderer.on('core:lifecycle', handler)
    return () => ipcRenderer.removeListener('core:lifecycle', handler)
  },
  getLifecycle: () => ipcRenderer.invoke('core:getLifecycle'),
  saveHar: (json, defaultName) => ipcRenderer.invoke('har:save', json, defaultName),
}

contextBridge.exposeInMainWorld('api', api)
