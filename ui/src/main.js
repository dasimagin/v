import '@fontsource/jetbrains-mono/latin-400.css'
import '@fontsource/jetbrains-mono/latin-600.css'
import '@fontsource/jetbrains-mono/latin-400-italic.css'
import '@fontsource/jetbrains-mono/cyrillic-400.css'
import '@fontsource/jetbrains-mono/cyrillic-600.css'
import '@fontsource/jetbrains-mono/cyrillic-400-italic.css'
import { mount } from 'svelte'
import './style.css'
import App from './App.svelte'

export default mount(App, { target: document.getElementById('app') })
