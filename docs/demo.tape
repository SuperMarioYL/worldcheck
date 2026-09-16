# worldcheck demo tape — records the m1 happy path into
# assets/demo/worldcheck-demo.gif. Run docs/demo-record.sh instead of calling
# vhs directly: it builds the binary and stages a clean demo directory so the
# recording shows exactly what a fresh clone sees.

Set Width 1100
Set Height 580
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

# 2. Provision it: compose up, apply the seed, record the manifest.
Type "worldcheck up --world worlds/postgres-starter"
Enter
Wait@12

# 3. Status prints the services and the seeded-state fingerprint.
Type "worldcheck status --world worlds/postgres-starter"
Enter
Wait@2.5

# 4. The m1 acceptance: edit the seed, re-run up, the fingerprint changes.
Type "echo 'UPDATE tasks SET done = true WHERE project_id = 2;' >> worlds/postgres-starter/seed.sql"
Enter
Wait@1
Type "worldcheck up --world worlds/postgres-starter"
Enter
Wait@10

# 5. The recorded manifest is the auditable artifact.
Type "cat worlds/postgres-starter/manifest.json"
Enter
Wait@4

Sleep 1s
