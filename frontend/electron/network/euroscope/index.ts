import { WebContents } from 'electron'
import { EuroScopeSocket } from './EuroScopeSocket'
import { Ipc } from './Ipc'
import { MessageHandler } from './MessageHandler'
import EventHandler from './EventHandler'

export function createEuroScopeSocket(webContents: WebContents): {
  socket: EuroScopeSocket
  eventHandler: EventHandler
} {
  const ipc = new Ipc(webContents)
  const handler = new MessageHandler(ipc)
  const euroScopeSocket = new EuroScopeSocket(handler)
  const eventHandler = new EventHandler(euroScopeSocket, webContents)

  return { socket: euroScopeSocket, eventHandler }
}
