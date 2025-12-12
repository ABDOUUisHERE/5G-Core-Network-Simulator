#!/bin/sh
set -e

# Installer les dépendances si vendor manquant
if [ ! -d "vendor" ]; then
    echo "vendor folder not found. Running composer install..."
    composer install --no-dev --optimize-autoloader
fi

# Lancer Apache en avant-plan
apache2-foreground