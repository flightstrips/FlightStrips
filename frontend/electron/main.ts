import { app, BrowserWindow, ipcMain, Menu, shell } from 'electron'
import { createEuroScopeSocket } from './network/euroscope'
import path from 'node:path'
import { IpcChannelInterface } from './IPC/IpcChannelInterface'
import { EuroScopeSocket } from './network/euroscope/EuroScopeSocket'
import EventHandler from './network/euroscope/EventHandler'

// The built directory structure
//
// ├─┬─┬ dist
// │ │ └── index.html
// │ │
// │ ├─┬ dist-electron
// │ │ ├── main.js
// │ │ └── preload.js
// │
process.env.DIST = path.join(__dirname, '../dist')
process.env.PUBLIC = app.isPackaged
  ? process.env.DIST
  : path.join(process.env.DIST, '../public')

const VITE_DEV_SERVER_URL = process.env['VITE_DEV_SERVER_URL']

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
      icon: path.join(process.env.PUBLIC, 'icon.ico'),
      webPreferences: {
        nodeIntegration: false,
        contextIsolation: true,
        preload: path.join(__dirname, 'preload.js'),
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

    if (VITE_DEV_SERVER_URL) {
      this.mainWindow.loadURL(VITE_DEV_SERVER_URL)
    } else {
      this.mainWindow.loadFile(path.join(process.env.DIST, 'index.html'))
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
          { label: 'Apron' },
          { label: 'Tower' },
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
