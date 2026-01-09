import './style.css';
import './app.css';

import { createApp } from 'vue';

import App from './App.vue';
import { loadStoredTheme, setDocumentTheme } from './theme';

setDocumentTheme(loadStoredTheme() ?? 'light');

createApp(App).mount('#app');

