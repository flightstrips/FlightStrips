import { ipcRenderer } from 'electron'

export default {
  ready: () => {
    ipcRenderer.send('ready', {})
  },
}
