import type { PrintSheet } from '@/types/weighing';
import './ticket-sheet.css';

export function TicketSheet({ sheet }: { sheet: PrintSheet }) {
  return <div className="ticket-sheet-scroll" tabIndex={0} role="region" aria-label="纸质过磅单预览">
    <article className="weighing-ticket-sheet" aria-label="精简过磅单">
      <h2>{sheet.title}</h2>
      <p className="ticket-sheet-marks">{sheet.marks.join(' · ') || '\u00a0'}</p>
      <div className="ticket-sheet-rows">{sheet.rows.map((row, index) => <div className="ticket-sheet-row" key={index}>{row.map(field => <div className="ticket-sheet-field" key={field.label}><span>{field.label}：</span><strong>{field.value}</strong></div>)}</div>)}</div>
    </article>
  </div>;
}
