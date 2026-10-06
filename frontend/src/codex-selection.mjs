export const codexEffortLabels = { low: '低（推荐）', medium: '中', high: '高', xhigh: '更高', max: '最高', ultra: 'Ultra' };

export function codexSelection(value) {
  return { model: typeof value?.model === 'string' ? value.model.trim() : '',
    effort: Object.hasOwn(codexEffortLabels, value?.effort) ? value.effort : 'low' };
}

export function codexEfforts(options, model) {
  const selected = options?.models?.find(item => item.id === (model || options.defaultModel));
  const levels = (selected?.reasoning || []).filter(level => Object.hasOwn(codexEffortLabels, level));
  return levels.length ? levels : ['low', 'medium', 'high'];
}
