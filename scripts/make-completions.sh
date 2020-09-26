#!/usr/bin/env bash

# scripts/make-completions.sh [progname]

echo '#!/usr/bin/env bash' > "scripts/bash-completion/$1"
echo '#!/usr/bin/env zsh' > "scripts/zsh-completion/$1"

$1 --init-completion=bash >> "scripts/bash-completion/$1"
$1 --init-completion=zsh >> "scripts/zsh-completion/$1"
