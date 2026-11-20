/** 复制文本到剪贴板（Electron renderer 可用 navigator.clipboard）。 */
export function copyText(text: string): void {
  void navigator.clipboard?.writeText(text)
}
