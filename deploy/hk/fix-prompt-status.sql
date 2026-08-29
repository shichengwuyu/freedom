USE freedom;
UPDATE prompts SET status='approved' WHERE category='image';
SELECT category, status, COUNT(0) AS cnt FROM prompts GROUP BY category, status;
