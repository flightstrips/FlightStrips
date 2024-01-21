import { app, BrowserWindow, ipcMain, Menu, shell } from 'electron'
import { createEuroScopeSocket } from '../network/euroscope'
import { join, dirname } from 'node:path'
import { IpcChannelInterface } from '../IPC/IpcChannelInterface'
import { EuroScopeSocket } from '../network/euroscope/EuroScopeSocket'
import EventHandler from '../network/euroscope/EventHandler'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)

if (!app.requestSingleInstanceLock()) {
  app.quit()
  process.exit(0)
}

// The built directory structure
//
// ├─┬ dist-electron
// │ ├─┬ main
// │ │ └── index.js    > Electron-Main
// │ └─┬ preload
// │   └── index.mjs   > Preload-Scripts
// ├─┬ dist
// │ └── index.html    > Electron-Renderer
//
process.env.DIST_ELECTRON = join(__dirname, '../')
process.env.DIST = join(process.env.DIST_ELECTRON, '../dist')
process.env.PUBLIC = app.isPackaged
  ? process.env.DIST
  : join(process.env.DIST_ELECTRON, '../public')

const preload = join(__dirname, '../preload/index.mjs')
const url = process.env.VITE_DEV_SERVER_URL
const indexHtml = join(process.env.DIST, 'index.html')

class Main {
  private mainWindow: BrowserWindow | null = null
  private euroScopeScoket: EuroScopeSocket | null = null
  private eventHandler: EventHandler | null = null

  public init(ipcChannels: IpcChannelInterface[]) {
    app.on('ready', this.createWindows)
    app.on('window-all-closed', this.onWindowAllClosed)

    this.registerIpcChannels(ipcChannels)
  }

  private createWindows() {
    this.mainWindow = new BrowserWindow({
      icon: join(process.env.PUBLIC, 'icon.ico'),
      webPreferences: {
        preload: preload,
      },
      minWidth: 1920,
      minHeight: 1080,
    })

    this.mainWindow.webContents.on('did-finish-load', () => {
      this.mainWindow?.webContents.send(
        'main-process-message',
        new Date().toLocaleString(),
      )
    })
    this.mainWindow.setAspectRatio(16 / 9)

    if (url) {
      this.mainWindow.loadURL(url)
    } else {
      this.mainWindow.loadFile(indexHtml)
    }
    const menu = Menu.buildFromTemplate([
      {
        label: 'Views',
        submenu: [
          {
            label: 'Delivery',
            click: () =>
              this.mainWindow?.webContents.send('navigate', '/ekch/del'),
          },
          {
            label: 'Apron',
            click: () =>
              this.mainWindow?.webContents.send('navigate', '/ekch/gnd'),
          },
          {
            label: 'Tower',
            click: () =>
              this.mainWindow?.webContents.send('navigate', '/ekch/twr'),
          },
        ],
      },
      {
        label: 'Window',
        submenu: [
          { role: 'forceReload' },
          { role: 'togglefullscreen' },
          { role: 'toggleDevTools' },
        ],
      },
      {
        label: 'Misc',
        submenu: [{ label: 'VATCAN Event Code' }, { type: 'separator' }],
      },
      {
        role: 'help',

        submenu: [
          {
            label: 'Documentation',
            click: async () => {
              await shell.openExternal('https://docs.fstools.dk')
            },
          },
          { type: 'separator' },
          {
            label: 'Support',
            click: async () => {
              await shell.openExternal(
                'https://github.com/frederikrosenberg/FlightStrips/issues',
              )
            },
          },
        ],
      },
    ])
    Menu.setApplicationMenu(menu)

    const result = createEuroScopeSocket(this.mainWindow.webContents)
    this.eventHandler = result.eventHandler
    this.eventHandler.setupHandlers()
    this.euroScopeScoket = result.socket
    this.euroScopeScoket.start()
  }

  private onWindowAllClosed() {
    this.euroScopeScoket?.stop()
    this.mainWindow = null
    this.eventHandler = null
    app.quit()
  }

  private registerIpcChannels(ipcChannels: IpcChannelInterface[]) {
    ipcChannels.forEach((channel) =>
      ipcMain.on(channel.getName(), (event, request) =>
        channel.handle(event, request),
      ),
    )
  }
}

const main: Main = new Main()
main.init([])
