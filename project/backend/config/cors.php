<?php

return [
    /*
    |--------------------------------------------------------------------------
    | Cross-Origin Resource Sharing (CORS) Configuration
    |--------------------------------------------------------------------------
    */

    'paths' => ['api/*', 'sanctum/csrf-cookie'],

    'allowed_methods' => ['*'],

    // ✅ CORRECTION SEC-003 : Limiter aux domaines autorisés uniquement
    'allowed_origins' => [
        'http://localhost:3000',  // Frontend React dev
        'http://localhost:5173',  // Alternative Vite
        // En production, ajouter : 'https://votredomaine.com'
    ],

    'allowed_origins_patterns' => [],

    'allowed_headers' => ['*'],

    'exposed_headers' => [],

    'max_age' => 0,

    'supports_credentials' => false,
];