const path = require('path');
const { menubar } = require('menubar');

const { app, BrowserWindow, Menu } = require('electron');
const isDev = require('electron-is-dev');
const mb = menubar



function createWindow() {
  // Create the browser window.
  const win = new BrowserWindow({
    title: "VATSCA FlightStrips V0.0.1",
    width: 1280,
    height: 720,
    webPreferences: {
      nodeIntegration: true,
    }
  });

  const template = [
    {
      role: 'viewMenu',
      submenu: [
        {
          label: 'Home',
          click: () => win.webContents.send('navigate', '/'),
        },
        {
          label: 'About',
          click: () => win.webContents.send('navigate', '/about'),
        },
      ],
    },
    {
      role: 'viewMenu',
      submenu: [
        {
          label: 'Kastrup Delivery',
          click: () => win.webContents.send('navigate', '/ekch/del'),
        },
        {
          label: 'About',
          click: () => win.webContents.send('navigate', '/about'),
        },
      ],
    },
    {
      role: 'help',
      submenu: [
        {
          label: 'Github Project',
          click: async () => {
            const { shell } = require('electron')
            await shell.openExternal('https://github.com/frederikrosenberg/FlightStrips')
          }
        },
        { type: 'separator' },
        { label: 'Version 0.0.0-alpha', enabled: 'TRUE' }
      ]
    }
  ]

  Menu.setApplicationMenu(Menu.buildFromTemplate(template))

  // and load the index.html of the app.
  // win.loadFile("index.html");
  win.loadURL(
    isDev
      ? 'http://localhost:3000'
      : `file://${path.join(__dirname, '../build/index.html')}`
  );
  // Open the DevTools.
  if (isDev) {
    win.webContents.openDevTools({ mode: 'detach' });
  }
}

// This method will be called when Electron has finished
// initialization and is ready to create browser windows.
// Some APIs can only be used after this event occurs.
app.whenReady().then(() => {
  createWindow()
  
})


// Quit when all windows are closed, except on macOS. There, it's common
// for applications and their menu bar to stay active until the user quits
// explicitly with Cmd + Q.
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  }
});
