DELETE FROM ops_custom_services
WHERE lower(trim(name)) IN ('sentinel', 'sentinel-updater')
   OR lower(trim(unit)) IN (
       'sentinel',
       'sentinel.service',
       'sentinel-updater.timer',
       'io.opusdomini.sentinel',
       'io.opusdomini.sentinel.updater'
   );
