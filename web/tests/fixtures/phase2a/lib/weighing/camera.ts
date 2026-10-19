import type { CameraPreset } from '../../types/weighing';

export const cameraPresets: { id: CameraPreset; title: string; src: string }[] = [
  { id: 'front', title: '车头视角', src: '/demo-camera/truck-front.png' },
  { id: 'rear', title: '车尾视角', src: '/demo-camera/truck-rear.png' },
];

export function getCameraPreset(id?: string) {
  return cameraPresets.find((preset) => preset.id === id) ?? cameraPresets[0];
}
