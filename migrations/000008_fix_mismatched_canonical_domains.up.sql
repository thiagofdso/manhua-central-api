-- Fix canonical url_template for after-rebirth-i-used-mirror-reversal-for-vengeance
UPDATE canonical_manhuas
SET url_template = 'https://arenascan.com/after-rebirth-i-used-mirror-reversal-for-vengeance-chapter-{chapter}/'
WHERE slug = 'after-rebirth-i-used-mirror-reversal-for-vengeance';

-- Synchronize primary_domain with actual host in url_template for all canonical manhuas
UPDATE canonical_manhuas
SET primary_domain = CASE
    WHEN url_template LIKE '%asurascans.com%' THEN 'asurascans.com'
    WHEN url_template LIKE '%ravenscans.org%' THEN 'ravenscans.org'
    WHEN url_template LIKE '%mangak.io%' THEN 'mangak.io'
    WHEN url_template LIKE '%manhuatop.org%' THEN 'manhuatop.org'
    WHEN url_template LIKE '%mgeko.cc%' THEN 'mgeko.cc'
    WHEN url_template LIKE '%mangafire.to%' THEN 'mangafire.to'
    WHEN url_template LIKE '%comix.to%' THEN 'comix.to'
    WHEN url_template LIKE '%arenascan.com%' OR url_template LIKE '%arenascans.com%' THEN 'arenascan.com'
    WHEN url_template LIKE '%manhuaus.com%' THEN 'manhuaus.com'
    WHEN url_template LIKE '%magicemperor.online%' THEN 'magicemperor.online'
    WHEN url_template LIKE '%pickmeupgacha.com%' THEN 'pickmeupgacha.com'
    WHEN url_template LIKE '%levelingwithgods.com%' THEN 'levelingwithgods.com'
    ELSE primary_domain
END;
