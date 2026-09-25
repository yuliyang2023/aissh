import { createRoot } from 'react-dom/client';
import { App } from './App';
import { isMac } from './platform';
import './style.css';
document.documentElement.dataset.platform = isMac ? 'mac' : 'other';
createRoot(document.getElementById('root')!).render(<App />);
