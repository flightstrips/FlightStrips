import { app, BrowserWindow, Menu, shell } from 'electron'
import euroScope from 'euroscope-ts'
import path from 'node:path'

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
process.env.PUBLIC = app.isPackaged ? process.env.DIST : path.join(process.env.DIST, '../public')


let win: BrowserWindow | null
// 🚧 Use ['ENV_NAME'] avoid vite:define plugin - Vite@2.x
const VITE_DEV_SERVER_URL = process.env['VITE_DEV_SERVER_URL']

function createWindow() {
  win = new BrowserWindow({
    icon: path.join(process.env.PUBLIC, 'electron-vite.svg'),
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
    },
    minWidth:1920,
    minHeight:1080,
  })

  win.webContents.on('did-finish-load', () => {
    win?.webContents.send('main-process-message', (new Date).toLocaleString())
  })
  win.setAspectRatio(16/9)

  if (VITE_DEV_SERVER_URL) {
    win.loadURL(VITE_DEV_SERVER_URL)
  } else {
    win.loadFile(path.join(process.env.DIST, 'index.html'))
  }
  var menu = Menu.buildFromTemplate([
    {
      label: 'Views',
      submenu : [
        {label: 'Clearance Delivery',},
        {label: 'Apron'},
        {label: 'Tower'}
      ]
    },
    {
      label: 'Development',
      submenu : [
        {label: 'VATSIM Scandinavia',click(){shell.openExternal('https://vatsim-scandinavia.org/')}},
        {label: 'Github',click(){shell.openExternal('https://github.com/frederikrosenberg/FlightStrips')}},
        {label: 'Discord',click(){shell.openExternal('https://discord.gg/vatsca')}},
      ]
    }
  ])
  Menu.setApplicationMenu(menu)
}

app.on('window-all-closed', () => {
  euroScope.disconnect()
  win = null
})

app.whenReady().then(createWindow)

euroScope.connect()
