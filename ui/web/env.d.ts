/// <reference types="vite/client" />
import type { RendererApi } from './src/types'

declare global {
  interface Window {
    api: RendererApi
  }
}

export {}
