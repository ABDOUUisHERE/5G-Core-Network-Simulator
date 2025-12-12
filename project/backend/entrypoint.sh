#!/bin/bash
set -e

echo "=== Starting Laravel Application ==="

# Option 1: Vérifier la DB avec netcat si disponible
if command -v nc > /dev/null 2>&1; then
    echo "Waiting for database connection..."
    for i in {1..30}; do
        if nc -z db 3306 2>/dev/null; then
            echo "Database is ready!"
            break
        fi
        if [ $i -eq 30 ]; then
            echo "Warning: Database not ready after 30 attempts"
        fi
        sleep 2
    done
fi

# Option 2: Vérifier la DB avec mysqladmin si netcat n'est pas disponible
if command -v mysqladmin > /dev/null 2>&1; then
    echo "Checking database with mysqladmin..."
    if mysqladmin ping -h db -u root -psecret --silent; then
        echo "Database connection successful"
    else
        echo "Database connection failed, but continuing..."
    fi
fi

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
