import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@evoapi/design-system/styles';
import './styles/globals.css';
import App from './App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
