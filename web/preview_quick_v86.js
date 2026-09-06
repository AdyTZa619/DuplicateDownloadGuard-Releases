// Compatibility bootstrap retained because index.html still references this legacy filename.
// Actual features live in focused modules so provider work no longer grows the MEGA preview file.
(() => {
  'use strict';

  const modules = [
    ['/preview_quick_core.js', 'ddgPreviewQuickCore'],
    ['/provider_compare_ui_v8558.js', 'ddgProviderCompareUIV8558'],
    ['/provider_buffer_v8561.js', 'ddgProviderBufferV8561'],
    ['/provider_sources.js', 'ddgProviderSources'],
    ['/source_link_persistence_v8553.js', 'ddgSourceLinkPersistenceV8553Script'],
    ['/updater_resilience.js', 'ddgUpdaterResilience'],
    ['/download_actions_v8545.js', 'ddgDownloadActionsV8545Script'],
    ['/update_channels_v8546.js', 'ddgUpdateChannelsV8546'],
    ['/update_corner_hotfix_v8547.js', 'ddgUpdateCornerHotfixV8547'],
    ['/update_sound_v8552.js', 'ddgUpdateSoundV8552Script'],
    ['/jdownloader_final_v8551.js', 'ddgJDownloaderFinalV8551Script']
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
