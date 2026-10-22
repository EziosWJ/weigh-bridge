import { useEffect, useState } from 'react';
import { Alert, App, Button, Card, Drawer, Form, Grid, Input, Layout, Menu, Modal, Space, Tag, Tooltip, Typography } from 'antd';
import { Menu as MenuIcon, Moon, Search, Sun } from 'lucide-react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { appConfig } from '../config/app.config';
import { navigation } from '../config/navigation';
import { useMaintenance } from '../app/AppProviders';
import { MaintenanceActions, useActions } from '../app/MaintenanceActions';
import { keys, post, SessionExpired } from '../api';
import { PageHeader } from '../components/PageHeader/PageHeader';
import styles from './AppLayout.module.css';

export function AppLayout() {
  return <MaintenanceActions><Shell /></MaintenanceActions>;
}
function Shell() {
  const { authenticated, setAuthenticated, themeMode, toggleTheme } = useMaintenance();
  const { busy, message, run } = useActions();
  const { message: toast } = App.useApp();
  const [form] = Form.useForm();
  const [loginError, setLoginError] = useState('');
  const [mobileOpen, setMobileOpen] = useState(false);
  const [searchOpen, setSearchOpen] = useState(false);
  const [search, setSearch] = useState('');
  const screens = Grid.useBreakpoint();
  const isMobile = !screens.lg;
  const location = useLocation();
  const navigate = useNavigate();
  const client = useQueryClient();
  const current = navigation.find((item) => item.path === location.pathname) || navigation[0];
  useEffect(() => { document.title = `${current.title} - ${appConfig.name}`; window.scrollTo(0, 0); }, [current]);
  useEffect(() => {
    const shortcut = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') { event.preventDefault(); setSearchOpen(true); }
    };
    window.addEventListener('keydown', shortcut); return () => window.removeEventListener('keydown', shortcut);
  }, []);
  const go = (path: string) => { navigate(path); setMobileOpen(false); setSearchOpen(false); setSearch(''); };
  const items = navigation.map(({ path, label, icon: Icon }) => ({ key: path, label: <Link to={path} onClick={() => setMobileOpen(false)}>{label}</Link>, icon: <Icon size={16} aria-hidden /> }));
  const menu = (mode: 'horizontal' | 'inline') => <Menu mode={mode} selectedKeys={[current.path]} items={items}
    className={mode === 'horizontal' ? styles.desktopMenu : styles.mobileMenu} aria-label="主菜单" />;
  const themeButton = <Tooltip title={themeMode === 'dark' ? '切换浅色主题' : '切换深色主题'}><Button type="text"
    aria-label={themeMode === 'dark' ? '切换浅色主题' : '切换深色主题'} icon={themeMode === 'dark' ? <Sun size={18} /> : <Moon size={18} />} onClick={toggleTheme} /></Tooltip>;
  return <Layout className={styles.shell}>
    <a href="#main-content" className={styles.skipLink} onClick={(event) => { event.preventDefault(); document.getElementById('main-content')?.focus(); }}>跳到页面内容</a>
    <header className={styles.header}>
      <div className={styles.brand}>
        {isMobile && <Button type="text" aria-label="打开菜单" icon={<MenuIcon size={20} />} onClick={() => setMobileOpen(true)} />}
        <Link to="/overview" className={styles.brandLink}><span className={styles.brandMark}>磅</span><Typography.Text strong className={styles.brandName}>{appConfig.name}</Typography.Text></Link>
      </div>
      <nav className={styles.desktopNav} aria-label="主菜单">{!isMobile && menu('horizontal')}</nav>
      <Space size={4} className={styles.tools}>
        <Button aria-label="搜索页面" icon={<Search size={16} />} onClick={() => setSearchOpen(true)} />
        {themeButton}
        {authenticated && <Button disabled={busy} size="small" onClick={() => run(async () => {
          await post('/api/logout', {}); setAuthenticated(false);
        })}>退出维护登录</Button>}
      </Space>
    </header>
    <main id="main-content" tabIndex={-1} className={styles.content}>
      <PageHeader title={current.title} description={current.description} titleSuffix={<Tag>{authenticated ? '维护会话已验证' : '只读访问'}</Tag>}
        extra={<Button disabled={busy} onClick={() => {
          void client.invalidateQueries({ queryKey: keys.status });
          void client.invalidateQueries({ queryKey: ['private'] });
        }}>刷新</Button>} />
      <div id="operationMessage" role="status" aria-live="polite">{message && <Alert type={message.failed ? 'error' : 'success'} showIcon title={message.text} className="feedback" />}</div>
      {!authenticated && <Card title="维护登录" className="login-card">
        <p>状态摘要可直接查看。配置、完整日志及维护操作需要本地维护密码。</p>
        <Form form={form} layout="inline" onFinish={({ password }: { password: string }) => run(async () => {
          try { await post('/api/login', { password }); form.resetFields(); setLoginError(''); setAuthenticated(true); }
          catch (error) { setLoginError(error instanceof Error ? error.message : String(error)); if (!(error instanceof SessionExpired)) void toast.error('维护登录失败'); }
        })}>
          <Form.Item name="password" label="本地维护密码" rules={[{ required: true, message: '请输入本地维护密码' }]}><Input.Password autoComplete="current-password" /></Form.Item>
          <Button htmlType="submit" type="primary" loading={busy}>验证并登录</Button>
        </Form>
        {loginError && <p role="alert" className="error-text">{loginError}</p>}
      </Card>}
      <Outlet />
    </main>
    <footer className={styles.footer}>{appConfig.footerText}</footer>
    <Drawer title={appConfig.name} placement="left" open={mobileOpen && isMobile} onClose={() => setMobileOpen(false)} size={300}>
      {menu('inline')}{themeButton}
    </Drawer>
    <Modal title="搜索页面" open={searchOpen} onCancel={() => setSearchOpen(false)} footer={null} destroyOnHidden>
      <Input autoFocus aria-label="搜索页面名称" value={search} onChange={(event) => setSearch(event.target.value)} onPressEnter={() => {
        const match = navigation.find((item) => (item.label + item.title).includes(search)); if (match) go(match.path);
      }} placeholder="输入页面名称，按 Enter 跳转" />
      <div className="search-results">{navigation.filter((item) => (item.label + item.title).includes(search)).map((item) =>
        <Button key={item.path} block type="text" onClick={() => go(item.path)}>{item.title}</Button>)}</div>
    </Modal>
  </Layout>;
}
