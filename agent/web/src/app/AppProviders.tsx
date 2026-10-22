import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { App, ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import { QueryCache, QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { getInitialTheme, getStoredTheme, getSystemTheme, storeTheme, type ThemeMode } from '../utils/theme';
import { createThemeConfig } from '../styles/theme';
import { request, SessionExpired } from '../api';

interface AppContext {
  authenticated: boolean;
  setAuthenticated: (value: boolean) => void;
  themeMode: ThemeMode;
  toggleTheme: () => void;
}
const Context = createContext<AppContext | null>(null);
export function useMaintenance() {
  const value = useContext(Context);
  if (!value) throw Error('Missing maintenance provider');
  return value;
}

export function AppProviders({ children }: { children: ReactNode }) {
  const [authenticated, setAuthenticated] = useState(false);
  const [themeMode, setThemeMode] = useState<ThemeMode>(getInitialTheme);
  const [client] = useState(() => new QueryClient({
    queryCache: new QueryCache({ onError: (error) => {
      if (error instanceof SessionExpired) setAuthenticated(false);
    } }),
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  }));
  useEffect(() => {
    let active = true;
    request<{ authenticated: boolean }>('/api/session').then((session) => {
      if (active) setAuthenticated(session.authenticated);
    }).catch(() => { if (active) setAuthenticated(false); });
    return () => { active = false; };
  }, []);
  useEffect(() => {
    if (!authenticated) {
      void client.cancelQueries({ queryKey: ['private'] });
      client.removeQueries({ queryKey: ['private'] });
    }
  }, [authenticated, client]);
  useEffect(() => {
    document.documentElement.dataset.theme = themeMode;
    document.documentElement.style.colorScheme = themeMode;
  }, [themeMode]);
  useEffect(() => {
    const media = window.matchMedia('(prefers-color-scheme: dark)');
    const follow = () => { if (getStoredTheme() === null) setThemeMode(getSystemTheme()); };
    media.addEventListener('change', follow);
    return () => media.removeEventListener('change', follow);
  }, []);
  const value = useMemo(() => ({
    authenticated, setAuthenticated, themeMode,
    toggleTheme: () => setThemeMode((mode) => {
      const next = mode === 'dark' ? 'light' : 'dark'; storeTheme(next); return next;
    }),
  }), [authenticated, themeMode]);
  return <QueryClientProvider client={client}><Context.Provider value={value}>
    <ConfigProvider locale={zhCN} theme={createThemeConfig(themeMode)} button={{ autoInsertSpace: false }}>
      <App>{children}</App>
    </ConfigProvider>
  </Context.Provider></QueryClientProvider>;
}
