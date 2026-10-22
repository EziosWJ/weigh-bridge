import { theme as antdTheme, type ThemeConfig } from 'antd';
import { appConfig } from '../config/app.config';
import type { ThemeMode } from '../utils/theme';

export function createThemeConfig(mode: ThemeMode): ThemeConfig {
  const isDark = mode === 'dark';
  // The brand purple is too dark for text on the dark surfaces; use a lighter
  // link/selected colour while keeping the same hue in light mode.
  const linkColor = isDark ? '#b7afff' : appConfig.brandColor;
  return {
    algorithm: isDark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    cssVar: {},
    token: {
      colorPrimary: appConfig.brandColor,
      colorInfo: appConfig.brandColor,
      colorLink: linkColor,
      colorLinkHover: isDark ? '#d3ceff' : '#8b7cf6',
      colorLinkActive: isDark ? '#9d93ff' : '#4b44b8',
      borderRadius: 8,
      fontFamily:
        'Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif',
      fontSize: 14,
      colorBgLayout: isDark ? '#0f1117' : '#f4f5fa',
      colorBgContainer: isDark ? '#171922' : '#ffffff',
      colorBorderSecondary: isDark ? '#262a36' : '#eceef4',
      boxShadowTertiary: isDark
        ? '0 1px 2px rgb(0 0 0 / 24%)'
        : '0 1px 2px rgb(24 26 40 / 5%)',
    },
    components: {
      Layout: {
        headerBg: isDark ? '#171922' : '#ffffff',
        bodyBg: isDark ? '#0f1117' : '#f5f6fb',
        footerBg: 'transparent',
      },
      Menu: {
        itemBorderRadius: 8,
        activeBarBorderWidth: 0,
        itemSelectedColor: linkColor,
        horizontalItemSelectedColor: linkColor,
      },
      Table: {
        headerBg: isDark ? '#1c1f2c' : '#f7f8fc',
        headerColor: isDark ? '#e6e8f0' : '#4a4e5e',
        headerSplitColor: 'transparent',
        rowHoverBg: isDark ? '#202433' : '#f8f7ff',
        cellPaddingBlock: 12,
        cellPaddingInline: 14,
        headerBorderRadius: 10,
      },
      Card: { borderRadiusLG: 12 },
      Button: { borderRadius: 8, controlHeight: 32 },
      Input: { controlHeight: 32 },
      Select: { controlHeight: 32 },
      DatePicker: { controlHeight: 32 },
      Tag: { borderRadiusSM: 6 },
      Drawer: { paddingLG: 24 },
    },
  };
}
