const secretConfigKeys = ['agentKey', 'cameraFrontUrl', 'cameraRearUrl', 'plateRecognitionUrl'];

export function safeDraft(config: Record<string, unknown>) {
  return Object.fromEntries(Object.entries(config).map(([key, value]) => [key, secretConfigKeys.includes(key) ? '' : value]));
}
