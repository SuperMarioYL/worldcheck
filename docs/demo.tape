# worldcheck demo tape — records the provisioning + checkpoint/diff path into
# assets/demo/worldcheck-demo.gif. Run docs/demo-record.sh instead of calling
# vhs directly: it builds the binary and stages a clean demo directory so the
# recording shows exactly what a fresh clone sees.

Output assets/demo/worldcheck-demo.gif

Set Width 1100
Set Height 620
Set FontSize 19
Set LetterSpacing 0.5
Set Theme { "name": "worldcheck", "black": "#050a12", "red": "#e06c75", "green": "#7ec9a4", "yellow": "#e5c07b", "blue": "#56ace6", "magenta": "#6a72e7", "cyan": "#acf3f6", "white": "#eefdfd", "brightBlack": "#95aec4", "brightWhite": "#ffffff" }
Set Padding 28
Set Shell zsh
Set TypingSpeed 45

Require worldcheck
Require git

Hide
Type "cd ~/worldcheck-demo && export PS1='demo $ ' && clear"
Enter
Show

# 1. The seeded test world ships with the repo: compose.yml + seed.sql.
Type "ls -l worlds/postgres-starter"
Enter
Wait@1.5

# 2. Provision it: compose up, wait for the database, apply the seed, record the manifest.
Type "worldcheck up --world worlds/postgres-starter"
Enter
Wait@15

# 3. Checkpoint the world before verification.
Type "worldcheck checkpoint t1 --world worlds/postgres-starter"
Enter
Wait@3

# 4. An agent bends its own world to green: one seeded row, one file it drops.
Type "docker compose -p wc-postgres-starter exec -T db psql -U postgres -d app -q -c 'UPDATE tasks SET done = true WHERE id = 6;'"
Enter
Wait@2.5
Type "docker compose -p wc-postgres-starter exec -T db sh -c 'echo bent by the agent > /tmp/agent-note'"
Enter
Wait@2

# 5. The diff catches exactly what the agent did.
Type "worldcheck diff t1 --world worlds/postgres-starter"
Enter
Wait@5

# 6. The m1 acceptance still holds: edit the seed, re-run up, the fingerprint moves.
Type "echo 'UPDATE tasks SET done = true WHERE project_id = 2;' >> worlds/postgres-starter/seed.sql"
Enter
Wait@1
Type "worldcheck up --world worlds/postgres-starter"
Enter
Wait@12

Sleep 1s
