import { app, BrowserWindow, ipcMain, Menu, shell } from 'electron'
import { createEuroScopeSocket } from './network/euroscope'
import path from 'node:path'
import { IpcChannelInterface } from './IPC/IpcChannelInterface'
import { EuroScopeSocket } from './network/euroscope/EuroScopeSocket'

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

  public init(ipcChannels: IpcChannelInterface[]) {
    app.on('ready', this.createWindows)
    app.on('window-all-closed', this.onWindowAllClosed)

    this.registerIpcChannels(ipcChannels)
  }

  private createWindows() {
    this.mainWindow = new BrowserWindow({
      icon: path.join(process.env.PUBLIC, 'electron-vite.svg'),
      webPreferences: {
        nodeIntegration: false,
        contextIsolation: true,
        preload: path.join(__dirname, 'preload.js'),
      },
      minWidth: 1920,
      minHeight: 1080,
    })

    this.mainWindow.webContents.openDevTools({ mode: 'undocked' })
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
          { label: 'Clearance Delivery' },
          { label: 'Apron' },
          { label: 'Tower' },
        ],
      },
      {
        label: 'Development',
        submenu: [
          {
            label: 'VATSIM Scandinavia',
            click() {
              shell.openExternal('https://vatsim-scandinavia.org/')
            },
          },
          {
            label: 'Github',
            click() {
              shell.openExternal(
                'https://github.com/frederikrosenberg/FlightStrips',
              )
            },
          },
          {
            label: 'Discord',
            click() {
              shell.openExternal('https://discord.gg/vatsca')
            },
          },
        ],
      },
    ])
    Menu.setApplicationMenu(menu)

    this.euroScopeScoket = createEuroScopeSocket(this.mainWindow.webContents)
    this.euroScopeScoket.start()
  }

  private onWindowAllClosed() {
    this.euroScopeScoket?.stop()
    this.mainWindow = null
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
