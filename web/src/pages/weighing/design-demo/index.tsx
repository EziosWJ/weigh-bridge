import { Link, useSearchParams } from 'react-router-dom';
import { IndustrialDemo } from './industrial';
import { ObservationDemo } from './observation';
import { BrightDemo } from './bright';
import './preview.css';

const designs = [
  { id: 'a', title: '工业仪表台', component: IndustrialDemo },
  { id: 'b', title: '现场监看台', component: ObservationDemo },
  { id: 'c', title: '明亮作业台', component: BrightDemo },
];

export function WorkbenchDesignDemoPage() {
  const [params] = useSearchParams();
  const selected = designs.find((design) => design.id === params.get('scheme')) ?? designs[0];
  const Demo = selected.component;

  return (
    <div className="workbench-design-preview">
      <nav className="wdp-switcher" aria-label="工作台设计方案">
        <span className="wdp-label">工作台设计预览</span>
        <div className="wdp-options">
          {designs.map((design) => (
            <Link
              key={design.id}
              to={`?scheme=${design.id}`}
              aria-current={selected.id === design.id ? 'page' : undefined}
              className={`wdp-option${selected.id === design.id ? ' wdp-option-selected' : ''}`}
            >
              <span>{design.id.toUpperCase()}</span>{design.title}
            </Link>
          ))}
        </div>
        <span className="wdp-note">静态页面 · 按钮仅作样式展示</span>
      </nav>
      <Demo key={selected.id} />
    </div>
  );
}
