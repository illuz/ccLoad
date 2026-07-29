(function () {
  'use strict';

  const REFRESH_INTERVAL_MS = 15000;
  const key = new URLSearchParams(window.location.search).get('key');
  const refreshButton = document.getElementById('refreshButton');
  const todayMetrics = document.getElementById('todayMetrics');
  const updatedAt = document.getElementById('updatedAt');
  const chartElement = document.getElementById('usageChart');

  let loading = false;
  let usageChart = null;

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

  function costMetric(today, quota) {
    const limit = Number(quota.limit_usd);
    const percentage = Number(quota.usage_percentage);
    if (Number.isFinite(limit) && limit > 0 && Number.isFinite(percentage)) {
      return {
        label: '实际费用',
        value: currency(today.effective_cost),
        detail: `费用上限 ${currency(limit)} · 已使用 ${number(percentage, 1)}%`,
        progress: Math.max(0, Math.min(100, percentage)),
        cost: true
      };
    }
    return {
      label: '实际费用',
      value: currency(today.effective_cost),
      cost: true
    };
  }

  function renderMetrics(data) {
    const today = data.today || {};
    const quota = data.cost_quota || {};
    const metrics = [
      costMetric(today, quota),
      { label: '请求次数', value: number(today.request_count) },
      { label: '总 Token', value: number(today.total_tokens) }
    ];

    const fragment = document.createDocumentFragment();
    metrics.forEach((metric) => {
      const item = document.createElement('div');
      item.className = `public-usage-metric${metric.cost ? ' public-usage-metric--cost' : ''}`;
      const label = document.createElement('dt');
      const value = document.createElement('dd');
      label.textContent = metric.label;
      value.textContent = metric.value;
      item.append(label, value);

      if (metric.detail) {
        const detail = document.createElement('p');
        detail.className = 'public-usage-metric-detail';
        detail.textContent = metric.detail;
        item.appendChild(detail);
      }
      if (metric.progress !== undefined) {
        const track = document.createElement('div');
        const fill = document.createElement('span');
        track.className = 'public-usage-quota-track';
        fill.className = 'public-usage-quota-fill';
        fill.style.width = `${metric.progress}%`;
        track.appendChild(fill);
        item.appendChild(track);
      }
      fragment.appendChild(item);
    });
    todayMetrics.replaceChildren(fragment);
  }

  function chartColors() {
    const styles = getComputedStyle(document.documentElement);
    return {
      cost: styles.getPropertyValue('--warning-500').trim() || '#f59e0b',
      token: styles.getPropertyValue('--primary-400').trim() || '#3ca3ff',
      text: styles.getPropertyValue('--neutral-600').trim() || '#d1d5db',
      grid: styles.getPropertyValue('--surface-border').trim() || 'rgba(255,255,255,0.12)'
    };
  }

  function renderChart(points) {
    if (!window.echarts || !chartElement) return;
    if (!usageChart) usageChart = window.echarts.init(chartElement);

    const colors = chartColors();
    const timeFormat = new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit' });
    const labels = points.map((point) => timeFormat.format(new Date(point.timestamp)));
    const costs = points.map((point) => Number(point.effective_cost) || 0);
    const tokens = points.map((point) => Number(point.total_tokens) || 0);

    usageChart.setOption({
      animationDuration: 250,
      color: [colors.cost, colors.token],
      grid: { left: 12, right: 12, top: 48, bottom: 8, containLabel: true },
      legend: {
        top: 4,
        data: ['费用', 'Token'],
        textStyle: { color: colors.text }
      },
      tooltip: {
        trigger: 'axis',
        formatter(params) {
          const rows = params.map((item) => {
            const value = item.seriesName === '费用' ? currency(item.value) : number(item.value);
            return `${item.marker}${item.seriesName}: ${value}`;
          });
          return `${params[0]?.axisValue || ''}<br>${rows.join('<br>')}`;
        }
      },
      xAxis: {
        type: 'category',
        boundaryGap: true,
        data: labels,
        axisLine: { lineStyle: { color: colors.grid } },
        axisTick: { show: false },
        axisLabel: { color: colors.text, hideOverlap: true }
      },
      yAxis: [
        {
          type: 'value',
          name: '费用 (USD)',
          nameTextStyle: { color: colors.text },
          axisLabel: { color: colors.text },
          splitLine: { lineStyle: { color: colors.grid } }
        },
        {
          type: 'value',
          name: 'Token',
          nameTextStyle: { color: colors.text },
          axisLabel: { color: colors.text, formatter: (value) => number(value) },
          splitLine: { show: false }
        }
      ],
      series: [
        {
          name: '费用',
          type: 'line',
          yAxisIndex: 0,
          data: costs,
          showSymbol: false,
          lineStyle: { width: 2 }
        },
        {
          name: 'Token',
          type: 'bar',
          yAxisIndex: 1,
          data: tokens,
          barMaxWidth: 18
        }
      ]
    }, true);
  }

  function render(data) {
    renderMetrics(data);
    renderChart(Array.isArray(data.trend) ? data.trend : []);
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
      const query = new URLSearchParams({ key });
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

  refreshButton.addEventListener('click', load);
  window.addEventListener('resize', () => usageChart?.resize());
  window.setInterval(load, REFRESH_INTERVAL_MS);
  load();
})();
