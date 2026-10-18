import { useState } from 'react';
import { Field } from '@/components/common/field';
import { Button } from '@/components/ui/button';
import { Select } from '@/components/ui/select';
import { useWeighingStore } from '@/store/weighing-store';
import { plateError } from '@/lib/weighing/plate';
import type { DemoConfig } from '@/types/weighing';
import { PlateInput } from './plate-input';

export function RecognitionSettings() {
  const { data, dispatch } = useWeighingStore();
  const [scenePlate, setScene] = useState(data.config.scenePlate || '晋A12345');
  const [recognitionCandidate, setCandidate] = useState(data.config.recognitionCandidate || '晋A12346');
  const [recognitionScenario, setScenario] = useState<DemoConfig['recognitionScenario']>(data.config.recognitionScenario || 'correct');
  const [feedback, setFeedback] = useState('');
  return <form className="space-y-3 border-t border-border pt-4" onSubmit={async event => {
    event.preventDefault();
    const error = plateError(scenePlate) || (recognitionScenario === 'incorrect' ? plateError(recognitionCandidate) || (recognitionCandidate === scenePlate ? '错误场景的候选须与现场车牌不同' : '') : '');
    if (error) { setFeedback(error); return; }
    try { await dispatch({ type: 'config', patch: { scenePlate, recognitionCandidate, recognitionScenario } }); setFeedback('模拟识别设置已保存，请在工作台点击“模拟车辆到磅”。'); }
    catch (failure) { setFeedback(failure instanceof Error ? failure.message : '保存失败'); }
  }}>
    <h3 className="text-base font-semibold">模拟车牌识别</h3>
    <p className="text-sm text-text-secondary">车牌特写与车辆示例图片组合展示，非真实识别；同一现场车牌可重复演示首磅和回磅。</p>
    <div className="flex flex-wrap gap-2"><Button size="sm" onClick={() => { setScene('晋A12345'); setCandidate('晋A12346'); setScenario('correct'); }}>普通车牌示例</Button><Button size="sm" onClick={() => { setScene('苏AD12345'); setCandidate('苏AD12346'); setScenario('incorrect'); }}>新能源错识示例</Button></div>
    <div className="grid gap-4 lg:grid-cols-3">
      <Field label="模拟现场车牌" htmlFor="scene-plate" required><PlateInput id="scene-plate" value={scenePlate} onChange={setScene} /></Field>
      <Field label="模拟识别场景" htmlFor="recognition-scenario"><Select id="recognition-scenario" value={recognitionScenario} onChange={event => setScenario(event.target.value as DemoConfig['recognitionScenario'])}><option value="correct">正确识别（候选与现场一致）</option><option value="incorrect">错误识别（人工修正）</option><option value="failure">识别失败（人工录入）</option></Select></Field>
      {recognitionScenario === 'incorrect' && <Field label="模拟识别候选" htmlFor="candidate-plate" required><PlateInput id="candidate-plate" value={recognitionCandidate} onChange={setCandidate} /></Field>}
    </div>
    <Button type="submit" variant="primary">保存模拟识别设置</Button>
    {feedback && <p role="status" className="text-sm text-text-secondary">{feedback}</p>}
  </form>;
}
