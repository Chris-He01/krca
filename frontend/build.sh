#!/bin/bash
set -e

cd "$(dirname "$0")"

echo "==> Installing dependencies..."
npm install

echo "==> Building frontend..."
npm run build

echo "==> Done! Static files in frontend/out/"
ls -la out/
