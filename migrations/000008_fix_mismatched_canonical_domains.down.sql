-- Revert canonical url_template for after-rebirth-i-used-mirror-reversal-for-vengeance
UPDATE canonical_manhuas
SET url_template = 'https://manhuatop.org/manhua/after-rebirth-i-used-mirror-reversal-for-vengeance/chapter-{chapter}/'
WHERE slug = 'after-rebirth-i-used-mirror-reversal-for-vengeance';
