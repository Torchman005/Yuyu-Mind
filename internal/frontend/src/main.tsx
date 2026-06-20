import React from 'react'
import {createRoot} from 'react-dom/client'
import './style.css'
import App from './App'

const container = document.getElementById('root')

if (!container) {
    throw new Error('Root container was not found')
}

window.addEventListener('error', (event) => {
    showBootError(event.message)
})

window.addEventListener('unhandledrejection', (event) => {
    showBootError(String(event.reason))
})

function showBootError(message: string) {
    let node = document.querySelector<HTMLPreElement>('.boot-error')
    if (!node) {
        node = document.createElement('pre')
        node.className = 'boot-error'
        document.body.appendChild(node)
    }
    node.textContent = message
}

const root = createRoot(container)

root.render(<App/>)
