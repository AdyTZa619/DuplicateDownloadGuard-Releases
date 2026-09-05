// Compatibility bootstrap retained because index.html still references this legacy filename.
// Actual features live in focused modules so provider work no longer grows the MEGA preview file.
(() => {
  'use strict';

  const modules = [
    ['/preview_quick_core.js', 'ddgPreviewQuickCore'],
    ['/provider_sources.js', 'ddgProviderSources']
  ];

  for (const [src, id] of modules) {
    if (document.getElementById(id)) continue;
    const script = document.createElement('script');
    script.id = id;
    script.src = src;
    script.async = false;
    document.head.appendChild(script);
  }
})();