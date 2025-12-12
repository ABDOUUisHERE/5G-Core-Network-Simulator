<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;

return new class extends Migration
{
    /**
     * Run the migrations.
     */
    public function up(): void
    {
        // Convertir la table articles en utf8mb4 avec collation insensible aux accents
        DB::statement('ALTER TABLE articles CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci');
        
        // Convertir également les colonnes texte
        DB::statement('ALTER TABLE articles MODIFY title VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci');
        DB::statement('ALTER TABLE articles MODIFY content TEXT CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci');
    }

    /**
     * Reverse the migrations.
     */
    public function down(): void
    {
        DB::statement('ALTER TABLE articles CONVERT TO CHARACTER SET latin1');
        DB::statement('ALTER TABLE articles MODIFY title VARCHAR(255) CHARACTER SET latin1');
        DB::statement('ALTER TABLE articles MODIFY content TEXT CHARACTER SET latin1');
    }
};