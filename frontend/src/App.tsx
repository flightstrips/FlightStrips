import startupBackground from './assets/startup.png'
import vatscaLogo from './assets/Postive.svg'
import logo from './assets/logo.svg'
import './App.css'

function App() {
  return (
    <>
      <img src={logo} className="startup logo" alt="" />
      <img src={vatscaLogo} className="startup vatscaLogo" alt="" />
      <img src={startupBackground} className="startupBackground" alt="" />
    </>
  )
}

export default App
