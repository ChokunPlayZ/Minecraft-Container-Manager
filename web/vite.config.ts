import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';
import { tanstackRouter } from '@tanstack/router-plugin/vite';

const THEME_BOOTSTRAP = `(function(){try{var c=document.cookie.split(';').map(function(s){return s.trim()}).filter(function(s){return s.indexOf('mcm_theme=')===0})[0];var t=c?c.split('=')[1]:null;if(!t){try{t=window.localStorage.getItem('mcm-theme')}catch(e){}}var d=t==='dark'||((!t||t==='system')&&window.matchMedia&&window.matchMedia('(prefers-color-scheme: dark)').matches);document.documentElement.classList.toggle('dark',!!d);}catch(e){}})();`;

export default defineConfig({
  plugins: [themeBootstrapPlugin(), tanstackRouter({ target: 'react' }), react(), tailwindcss()],
});

// Inject the theme bootstrap script into the built HTML <head>.
export function themeBootstrapPlugin() {
  return {
    name: 'mcm-theme-bootstrap',
    transformIndexHtml: {
      order: 'pre' as const,
      handler: () => [
        {
          tag: 'script',
          attrs: { type: 'text/javascript' },
          children: THEME_BOOTSTRAP,
          injectTo: 'head' as const,
        },
      ],
    },
  };
}
