// Compatibility bootstrap retained because index.html still references this legacy filename.
// Actual features live in focused modules so provider work no longer grows the MEGA preview file.
(() => {
  'use strict';

  const modules = [
    ['/preview_quick_core.js', 'ddgPreviewQuickCore'],
    ['/provider_compare_ui_v8558.js', 'ddgProviderCompareUIV8558'],
    ['/provider_buffer_v8561.js', 'ddgProviderBufferV8561'],
    ['/provider_sources.js', 'ddgProviderSources'],
    ['/check_activity_v8563.js', 'ddgCheckActivityV8563Script'],
    ['/operation_hud_plus_v8565.js', 'ddgOperationHudPlusV8565Script'],
    ['/source_link_persistence_v8553.js', 'ddgSourceLinkPersistenceV8553Script'],
    ['/updater_resilience.js', 'ddgUpdaterResilience'],
    ['/download_actions_v8545.js', 'ddgDownloadActionsV8545Script'],
    ['/update_channels_v8546.js', 'ddgUpdateChannelsV8546'],
    ['/update_corner_hotfix_v8547.js', 'ddgUpdateCornerHotfixV8547'],
    ['/update_sound_v8552.js', 'ddgUpdateSoundV8552Script'],
    ['/update_fast_watch_v8562.js', 'ddgFastUpdateWatchV8562Script'],

    // Must load before every legacy JD capture listener. It owns the click,
    // uses current DDG rows only and performs exactly one FlashGot form POST.
    ['/jdownloader_fast_v8566.js', 'ddgJDownloaderFastV8566Script'],
    ['/media_picker_v8566.js', 'ddgMediaPickerV8566Script'],

    // Kept for backwards compatibility/tests, but their capture listeners are
    // later in registration order and therefore cannot steal JD clicks.
    ['/jdownloader_bunkr_compat_v8563.js', 'ddgJDownloaderBunkrCompatV8563Script'],
    ['/jdownloader_batch_confirm_v8564.js', 'ddgJDownloaderBatchConfirmV8564Script'],
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
