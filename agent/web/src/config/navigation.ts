import { Activity, Cable, FileClock, Printer, Settings } from 'lucide-react';

export const navigation = [
  { path: '/overview', label: '总览', title: '运行总览', description: '查看现场服务状态，定位连接与打印异常。', icon: Activity },
  { path: '/printing', label: '打印与队列', title: '打印与队列', description: '检查本机队列与未结束作业，按影响范围执行维护操作。', icon: Printer },
  { path: '/devices', label: '设备', title: '现场设备', description: '查看仪表实时状态与摄像头接入能力。', icon: Cable },
  { path: '/config', label: '配置', title: '连接、仪表与打印配置', description: '核对运行参数，保存草稿并明确应用。', icon: Settings },
  { path: '/records', label: '记录', title: '维护记录与日志', description: '按时间和操作查询本机维护记录，查看运行日志。', icon: FileClock },
];
