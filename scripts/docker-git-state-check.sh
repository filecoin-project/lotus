set -e

if [ -z "$(git status --porcelain)" ]; then
  echo "PASSED: Working directory clean"
else
  echo "FAILED: Working directory not clean."
  echo "This is likely because the .dockerignore file has removed something checked into git."
  echo "Add the missing files listed below to the .dockerignore to fix this issue:"
  echo "$(git status)"
  exit 1
fi

