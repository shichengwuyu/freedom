USE freedom;
SELECT DISTINCT category FROM prompts;
SELECT id, category, status FROM prompts LIMIT 10;
SELECT COUNT(0) AS total FROM prompts;
