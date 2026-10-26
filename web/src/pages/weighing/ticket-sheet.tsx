import type { PrintSheet } from '@/types/weighing';
import './ticket-sheet.css';

export function TicketSheet({ sheet }: { sheet: PrintSheet }) {
  const current = sheet.template === 'weighing-compact-v2';
  return <div className="ticket-sheet-scroll" tabIndex={0} role="region" aria-label="纸质过磅单预览">
    <article className={`weighing-ticket-sheet${current ? ' ticket-sheet-continuous' : ''}`} aria-label="精简过磅单">
      <div className="ticket-sheet-content">
        <h2>{sheet.title}</h2>
        <p className="ticket-sheet-marks">{sheet.marks.join(' · ') || '\u00a0'}</p>
        <div className="ticket-sheet-rows">{sheet.rows.map((row, index) => <div className={`ticket-sheet-row${current && index === 3 ? ' ticket-sheet-weights' : ''}${current && index === 5 ? ' ticket-sheet-signature' : ''}`} key={index}>{row.map(field => <div className="ticket-sheet-field" key={field.label}><span>{field.label}：</span><strong>{field.value}</strong></div>)}</div>)}</div>
      </div>
    </article>
  </div>;
}
