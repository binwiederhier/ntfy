import * as React from 'react';
import { createRoot } from 'react-dom/client';
import App from './components/App';

const root = createRoot(document.querySelector('#root'));
root.render(<App />);
