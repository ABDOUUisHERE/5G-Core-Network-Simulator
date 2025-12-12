#!/bin/bash
set -e

echo "=== Starting Laravel Application ==="

# Attendre simplement (sans vérification complexe)
echo "Waiting for services to be ready..."
sleep 15

# Vérifier les dépendances Composer
if [ ! -d "vendor" ] || [ ! -f "vendor/autoload.php" ]; then
    echo "Installing Composer dependencies..."
    composer install --optimize-autoloader --no-interaction
else
    echo "Composer dependencies already installed"
fi

# Configuration Laravel
echo "Setting up Laravel..."

# Créer .env si absent
if [ ! -f ".env" ]; then
    echo "Creating .env file from example..."
    cp .env.example .env
    php artisan key:generate
fi

# Nettoyer les caches
php artisan config:clear
php artisan cache:clear
php artisan route:clear
php artisan view:clear

# Exécuter les migrations
echo "Running database migrations..."
php artisan migrate --force --no-interaction

echo "=== Starting Apache ==="
exec apache2-foreground
