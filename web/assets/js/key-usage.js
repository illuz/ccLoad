(function () {
  'use strict';

  const REFRESH_INTERVAL_MS = 15000;
  const key = new URLSearchParams(window.location.search).get('key');
  const rangeSelect = document.getElementById('historyRange');
  const refreshButton = document.getElementById('refreshButton');
  const historyMetrics = document.getElementById('historyMetrics');
  const todayMetrics = document.getElementById('todayMetrics');
  const totalMetrics = document.getElementById('totalMetrics');
  const historyPeriod = document.getElementById('historyPeriod');
  const updatedAt = document.getElementById('updatedAt');

  let loading = false;

  if (!key) {
    window.location.replace('/key-usage');
    return;
  }

  function number(value, maximumFractionDigits) {
    return new Intl.NumberFormat('zh-CN', {
      maximumFractionDigits: maximumFractionDigits === undefined ? 0 : maximumFractionDigits
    }).format(Number(value) || 0);
  }

  function currency(value) {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 6
    }).format(Number(value) || 0);
  }

  function percent(stats) {
    const total = (Number(stats.success_count) || 0) + (Number(stats.failure_count) || 0);
    if (!total) return '0%';
    return `${number((Number(stats.success_count) || 0) * 100 / total, 1)}%`;
  }

  function milliseconds(seconds) {
    return `${number((Number(seconds) || 0) * 1000, 0)} ms`;
  }

  function metricsFor(stats, includeLive) {
    const metrics = [
      { label: '实际费用', value: currency(stats.effective_cost), cost: true },
      { label: '请求次数', value: number((Number(stats.success_count) || 0) + (Number(stats.failure_count) || 0)) },
      { label: '成功率', value: percent(stats) },
      { label: '输入 Token', value: number(stats.prompt_tokens) },
      { label: '输出 Token', value: number(stats.completion_tokens) },
      { label: '缓存读取', value: number(stats.cache_read_tokens) },
      { label: '缓存写入', value: number(stats.cache_creation_tokens) }
    ];

    if (includeLive) {
      metrics.push({ label: '最近一分钟 RPM', value: number(stats.recent_rpm, 1) });
    } else {
      metrics.push({ label: '平均 RPM', value: number(stats.avg_rpm, 1) });
    }
    return metrics;
  }

  function renderMetrics(container, metrics) {
    const fragment = document.createDocumentFragment();
    metrics.forEach((metric) => {
      const item = document.createElement('div');
      item.className = `public-usage-metric${metric.cost ? ' public-usage-metric--cost' : ''}`;
      const label = document.createElement('dt');
      const value = document.createElement('dd');
      label.textContent = metric.label;
      value.textContent = metric.value;
      item.append(label, value);
      fragment.appendChild(item);
    });
    container.replaceChildren(fragment);
  }

  function formatPeriod(start, end) {
    const format = new Intl.DateTimeFormat('zh-CN', {
      month: 'numeric',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    });
    return `${format.format(new Date(start))} - ${format.format(new Date(end))}`;
  }

  function render(data) {
    renderMetrics(historyMetrics, metricsFor(data.history || {}, data.history_is_current));
    renderMetrics(todayMetrics, metricsFor(data.today || {}, true));
    renderMetrics(totalMetrics, metricsFor(data.total || {}, false));
    historyPeriod.textContent = formatPeriod(data.range_start, data.range_end);
    updatedAt.textContent = `更新于 ${new Intl.DateTimeFormat('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    }).format(new Date(data.updated_at))}`;
  }

  async function load() {
    if (loading) return;
    loading = true;
    refreshButton.disabled = true;
    try {
      const query = new URLSearchParams({ key, range: rangeSelect.value });
      const response = await fetch(`/public/key-usage?${query.toString()}`, {
        headers: { Accept: 'application/json' },
        cache: 'no-store'
      });
      if (response.status === 404) {
        window.location.replace('/key-usage');
        return;
      }
      if (!response.ok) throw new Error(`request failed: ${response.status}`);
      const payload = await response.json();
      if (!payload.success || !payload.data) throw new Error('invalid response');
      render(payload.data);
    } catch (error) {
      updatedAt.textContent = '数据暂不可用';
    } finally {
      loading = false;
      refreshButton.disabled = false;
    }
  }

  rangeSelect.addEventListener('change', load);
  refreshButton.addEventListener('click', load);
  window.setInterval(load, REFRESH_INTERVAL_MS);
  load();
})();
