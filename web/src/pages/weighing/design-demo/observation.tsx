import './observation.css'

export const ObservationDemo = () => (
  <main className="observation-demo">
    <header className="odb-header">
      <div className="odb-brand">
        <span className="odb-mark" aria-hidden="true">磅</span>
        <div>
          <strong>现场监看台</strong>
          <span>东门地磅 · 1 号秤台</span>
        </div>
      </div>
      <div className="odb-header-right">
        <span className="odb-live"><i />连接正常</span>
        <span className="odb-clock">2026-10-18 14:32:08</span>
        <span className="odb-operator">操作员 王师傅</span>
      </div>
    </header>

    <section className="odb-workspace" aria-label="称重现场监看">
      <div className="odb-scene-column">
        <section className="odb-scene" aria-label="模拟摄像头画面">
          <img src="/demo-camera/truck-front.png" alt="蓝白货车停在地磅上的模拟画面" />
          <div className="odb-scene-top">
            <span className="odb-camera-name"><i />秤台主摄像头</span>
            <span className="odb-sim-tag">模拟画面 · 非现场抓拍</span>
          </div>
          <div className="odb-scene-bottom">
            <span>画面时间 14:32:08</span>
            <span>设备 CAM-01</span>
          </div>
          <span className="odb-corner odb-corner-tl" /><span className="odb-corner odb-corner-tr" />
          <span className="odb-corner odb-corner-bl" /><span className="odb-corner odb-corner-br" />
        </section>

        <div className="odb-evidence-row">
          <div className="odb-plate-crop">
            <div className="odb-crop-image">
              <img src="/demo-camera/truck-front.png" alt="车辆前部模拟特写" />
              <span>车头区域</span>
            </div>
            <div className="odb-plate-detail">
              <span>识别车牌 · 待人工核对</span>
              <strong>晋A<span>·</span>12345</strong>
            </div>
          </div>
          <div className="odb-capture-strip">
            <div className="odb-strip-heading"><strong>本次抓拍</strong><span>演示样图</span></div>
            <div className="odb-thumbnails">
              <figure><img src="/demo-camera/truck-front.png" alt="主视模拟抓拍" /><figcaption>主视 14:32:08</figcaption></figure>
            </div>
          </div>
        </div>
      </div>

      <aside className="odb-side-panel" aria-label="车辆和称重信息">
        <div className="odb-panel-heading">
          <div><span>当前车辆</span><h1>入厂卸货</h1></div>
          <span className="odb-step">首磅</span>
        </div>

        <div className="odb-plate-check">
          <label htmlFor="odb-plate">车牌核对</label>
          <div className="odb-plate-entry">
            <span className="odb-province">晋</span>
            <input id="odb-plate" value="A12345" readOnly aria-label="车牌号码" />
            <span className="odb-check-state">待核对</span>
          </div>
          <p>请对照左侧现场画面确认车牌</p>
        </div>

        <div className="odb-weight-block">
          <div className="odb-weight-label"><span>当前毛重</span><span className="odb-stability"><i />稳定</span></div>
          <div className="odb-weight-value">32.48<span>吨</span></div>
          <div className="odb-weight-meta"><span>地磅读数</span><span>更新于 14:32:08</span></div>
        </div>

        <div className="odb-vehicle-summary">
          <div><span>称重类型</span><strong>毛重 · 首次称重</strong></div>
          <div><span>物料</span><strong>酿酒高粱</strong></div>
          <div><span>供货方</span><strong>晋中丰谷粮贸有限公司</strong></div>
        </div>

        <div className="odb-panel-footer">
          <span>称重记录需由操作员确认后提交</span>
          <button type="button">采集重量</button>
        </div>
      </aside>
    </section>
  </main>
)

export default ObservationDemo
