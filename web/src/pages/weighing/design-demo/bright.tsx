import './bright.css'

const rows = [
  ['运输方向', '入厂卸货'],
  ['称重阶段', '首次称重 · 毛重'],
  ['物料', '酿酒高粱'],
  ['供货方', '晋中丰谷粮贸有限公司'],
  ['车牌号码', '晋A12345'],
  ['运输任务', 'RW-20261018-0286'],
]

export function BrightDemo() {
  return (
    <main className="bright-demo">
      <header className="bdc-topbar">
        <div className="bdc-brand"><span className="bdc-mark">磅</span><span>称重作业台</span></div>
        <div className="bdc-top-state"><span className="bdc-live-dot" />现场模拟画面 <span className="bdc-top-divider" /> 2026年10月18日 14:32:08</div>
        <div className="bdc-operator"><span className="bdc-avatar">王</span><span>王师傅</span><span className="bdc-muted">操作员</span></div>
      </header>

      <section className="bdc-weight-band" aria-label="当前车辆与称重">
        <div className="bdc-vehicle"><span className="bdc-plate">晋A12345</span><span className="bdc-direction">入厂卸货</span></div>
        <div className="bdc-weight-main"><div className="bdc-weight-label">当前毛重</div><div className="bdc-weight-value">32.48<span>吨</span></div></div>
        <div className="bdc-stability"><span className="bdc-stable-check">✓</span><span><b>重量稳定</b><small>保持稳定 3 秒</small></span></div>
        <div className="bdc-phase"><span>本次操作</span><b>首次称重</b><small>毛重记录</small></div>
      </section>

      <div className="bdc-content">
        <section className="bdc-photo-section">
          <div className="bdc-section-heading"><div><h1>车辆抓拍</h1></div><span className="bdc-camera-name"><i /> 一号地磅摄像头</span></div>
          <figure className="bdc-photo">
            <img src="/demo-camera/truck-front.png" alt="地磅上的货车模拟画面" />
            <figcaption><span className="bdc-sim-label"><i />模拟画面 · 非现场抓拍</span><span>14:32:08</span></figcaption>
          </figure>
          <div className="bdc-photo-note"><span className="bdc-note-icon">i</span><span>请核对车辆及车牌与本趟运输资料是否一致</span></div>
        </section>

        <section className="bdc-record-section">
          <div className="bdc-section-heading"><div><h2>本趟作业记录</h2></div><span className="bdc-record-status"><i />待操作员确认</span></div>
          <div className="bdc-record-table">
            {rows.map(([label, value], index) => <div className={`bdc-record-row${index === 4 ? ' bdc-plate-row' : ''}`} key={label}><span>{label}</span><b>{value}</b></div>)}
          </div>
          <div className="bdc-driver"><span className="bdc-driver-icon">人</span><span>驾驶员</span><b>李建国</b><span className="bdc-driver-phone">138****7621</span></div>
          <div className="bdc-confirm-note"><span className="bdc-confirm-rule" /><span>正式称重需由操作员核对资料并确认</span></div>
        </section>
      </div>

      <footer className="bdc-actionbar"><div className="bdc-action-hint"><span className="bdc-hint-icon">!</span><span>请先核对车辆和运输资料</span></div><div className="bdc-actions"><button className="bdc-button bdc-secondary" type="button">返回作业列表</button><button className="bdc-button bdc-primary" type="button">确认本次称重 <span>›</span></button></div></footer>
    </main>
  )
}
