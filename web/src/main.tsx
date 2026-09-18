import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { createAppRouter } from './router'
import './styles.css'

const router = createAppRouter()
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App router={router} />
  </StrictMode>,
)
