import { WebContents } from 'electron'
import { EuroScopeSocket } from './EuroScopeSocket'
import { Ipc } from './Ipc'
import { MessageHandler } from './MessageHandler'

export function createEuroScopeSocket(
  webContents: WebContents,
): EuroScopeSocket {
  const ipc = new Ipc(webContents)
  const handler = new MessageHandler(ipc)
  const euroScopeSocket = new EuroScopeSocket(handler)

  return euroScopeSocket
}
