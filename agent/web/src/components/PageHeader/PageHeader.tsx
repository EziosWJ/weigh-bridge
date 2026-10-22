import type { ReactNode } from 'react';
import { Space, Typography } from 'antd';
import styles from './PageHeader.module.css';

export interface PageHeaderProps {
  title: ReactNode;
  description?: ReactNode;
  extra?: ReactNode;
  meta?: ReactNode;
  titleSuffix?: ReactNode;
}

export function PageHeader({ title, description, extra, meta, titleSuffix }: PageHeaderProps) {
  return (
    <div className={styles.header}>
      <div className={styles.main}>
        <div className={styles.titleRow}>
          <Typography.Title level={1} className={styles.title}>
            {title}
          </Typography.Title>
          {titleSuffix}
        </div>
        {description && (
          <Typography.Text type="secondary" className={styles.description}>
            {description}
          </Typography.Text>
        )}
        {meta && <div className={styles.meta}>{meta}</div>}
      </div>
      {extra && (
        <Space wrap className={styles.extra}>
          {extra}
        </Space>
      )}
    </div>
  );
}
