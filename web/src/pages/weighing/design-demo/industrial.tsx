import './industrial.css'

const fields = [
  { label: '业务方向', value: '入厂卸货', hint: '首次称重 · 毛重' },
  { label: '物料', value: '酿酒高粱', hint: '原粮' },
  { label: '供货方', value: '晋中丰谷粮贸有限公司', hint: '供货主体' },
]

export function IndustrialDemo() {
  return (
    <main className="industrial-demo">
      <header className="ida-header">
        <div className="ida-brand">
          <div className="ida-mark" aria-hidden="true"><span /><span /><span /></div>
          <div>
            <div className="ida-title">磅房作业台</div>
            <div className="ida-subtitle">一号地磅 <span className="ida-dot" /> 本地演示界面</div>
          </div>
        </div>
        <div className="ida-header-right">
          <span className="ida-demo-tag"><i /> 静态演示</span>
          <div className="ida-clock"><strong>14:32:08</strong><span>2026年10月18日 星期日</span></div>
          <div className="ida-operator"><span className="ida-avatar">王</span><span>王师傅</span></div>
        </div>
      </header>

      <section className="ida-platebar" aria-label="当前车辆">
        <div className="ida-plate-label"><span className="ida-live-dot" />当前车辆</div>
        <div className="ida-plate" aria-label="车牌 晋 A12345">
          <span className="ida-province">晋</span><span className="ida-plate-divider" />
          <span className="ida-plate-number">A12345</span>
        </div>
        <span className="ida-plate-note">车牌已人工核对</span>
        <span className="ida-vehicle-type">普通货车</span>
        <div className="ida-plate-spacer" />
        <div className="ida-direction"><span className="ida-direction-icon">↓</span><span>入厂卸货</span></div>
        <div className="ida-ticket">任务号 <strong>RW-20261018-0286</strong></div>
      </section>

      <div className="ida-workarea">
        <section className="ida-scene" aria-label="现场观察">
          <div className="ida-section-heading">
            <div><span className="ida-section-bar" /><h2>现场观察</h2><span className="ida-muted">磅台摄像头 · 正向</span></div>
            <span className="ida-view-tag"><i />模拟画面</span>
          </div>
          <figure className="ida-camera">
            <img src="/demo-camera/truck-front.png" alt="模拟磅台画面：货车停在地磅上" />
            <div className="ida-camera-top"><span><i /> CAM 01 磅台正向</span><span>演示素材 · 非现场抓拍</span></div>
            <div className="ida-camera-corner ida-corner-tl" /><div className="ida-camera-corner ida-corner-tr" />
            <div className="ida-camera-corner ida-corner-bl" /><div className="ida-camera-corner ida-corner-br" />
            <div className="ida-camera-bottom"><span>车辆位置正常</span><span>视野覆盖磅台</span></div>
          </figure>
          <div className="ida-camera-footer">
            <div className="ida-camera-state"><span className="ida-state-mark">✓</span><span><strong>车辆完整入磅</strong><small>前后轮均在秤台范围内</small></span></div>
            <div className="ida-view-switch" aria-label="摄像头视角"><span className="ida-view-active">正向画面</span><span>后向画面</span></div>
          </div>
          <div className="ida-observation-note"><span className="ida-note-icon">i</span>示例车辆图片仅供界面展示，不代表实时现场或本次称重抓拍。</div>
        </section>

        <section className="ida-operation" aria-label="称重与业务信息">
          <div className="ida-gauge">
            <div className="ida-gauge-head"><div><span className="ida-gauge-kicker">当前秤台读数</span><span className="ida-reading-state"><i /> 稳定</span></div><span className="ida-device">地磅 01 <b>在线</b></span></div>
            <div className="ida-reading"><span className="ida-reading-value">32.48</span><span className="ida-reading-unit">吨</span></div>
            <div className="ida-gauge-bottom"><span>毛重</span><span className="ida-gauge-rule" /><span>采样稳定</span><span className="ida-gauge-signal"><i /><i /><i /><i /><i /><i /><i /></span></div>
            <div className="ida-gauge-meta"><span>重量采集时间</span><strong>14:32:05</strong><span className="ida-gauge-meta-divider" /><span>重量单位</span><strong>吨</strong></div>
          </div>

          <div className="ida-business-head"><h2>本趟业务信息</h2><span>运输任务</span></div>
          <div className="ida-fields">
            {fields.map((field) => (
              <div className="ida-field" key={field.label}>
                <span className="ida-field-label">{field.label}</span>
                <div className="ida-field-value"><strong>{field.value}</strong><small>{field.hint}</small></div>
              </div>
            ))}
            <div className="ida-field ida-driver-field"><span className="ida-field-label">运输车辆</span><div className="ida-field-value"><strong>晋A12345</strong><small>司机 李建国</small></div></div>
          </div>

          <div className="ida-action-area">
            <div className="ida-action-copy"><strong>等待操作员确认</strong><span>确认后记录本次毛重</span></div>
            <button className="ida-confirm" type="button"><span className="ida-confirm-check">✓</span>确认毛重</button>
          </div>
          <div className="ida-footnote"><span>i</span>当前为静态演示画面，不会生成正式称重记录。</div>
        </section>
      </div>
    </main>
  )
}
