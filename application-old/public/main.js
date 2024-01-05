const { app, BrowserWindow, ipcMain  } = require('electron')
const { mainMenu } = require("./menu")


function createWindow () {

  const win = new BrowserWindow({
    width: 1920,
    height: 1080,
    minHeight:1080,
    minWidth:1920,
    webPreferences: {
      nodeIntegration: true,
      enableRemoteModule:true,
    }
  })

  win.loadURL('http://localhost:3000');
  win.webContents.openDevTools()
  win.AspectRatio(1.77777777778);
  win.setApplicationMenu(mainMenu);
  ipcMain.on('navigate', (event, route) => {
    win.webContents.send('navigate', route);
  });
}



app.whenReady().then(createWindow)

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})

app.on('activate', () => {
  // On macOS it's common to re-create a window in the app when the
  // dock icon is clicked and there are no other windows open.
  
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow()
  }
})
