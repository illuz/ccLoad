(function () {
  const theme = 'dark';
  const resolvedTheme = 'dark';
  document.documentElement.dataset.theme = theme;
  document.documentElement.dataset.resolvedTheme = resolvedTheme;
  document.documentElement.style.colorScheme = resolvedTheme;
  document.documentElement.style.backgroundColor = '#0f172a';
  document.documentElement.style.color = '#e5e7eb';

  try {
    localStorage.setItem('ccload_theme', theme);
  } catch (_) {
    /* 忽略存储失败 */
  }

  const meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.setAttribute('content', '#0f172a');

  function clearInitialPaintStyle() {
    document.documentElement.style.removeProperty('background-color');
    document.documentElement.style.removeProperty('color');
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', clearInitialPaintStyle, { once: true });
  } else {
    requestAnimationFrame(clearInitialPaintStyle);
  }
})();
