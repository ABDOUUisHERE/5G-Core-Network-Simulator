<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Support\Facades\DB;
use Illuminate\Support\Facades\Hash;

class HashExistingPasswords extends Migration
{
    /**
     * Run the migrations.
     * Hash all existing plain-text passwords in the users table
     *
     * @return void
     */
    public function up()
    {
        // Get all users with plain-text passwords
        $users = DB::table('users')->get();

        foreach ($users as $user) {
            // Only hash if password doesn't already look hashed
            // Bcrypt hashes start with $2y$ and are 60 chars long
            if (!str_starts_with($user->password, '$2y$') && strlen($user->password) < 60) {
                DB::table('users')
                    ->where('id', $user->id)
                    ->update([
                        'password' => Hash::make($user->password),
                        'updated_at' => now(),
                    ]);
            }
        }
    }

    /**
     * Reverse the migrations.
     * WARNING: Cannot unhash passwords - this migration is irreversible
     *
     * @return void
     */
    public function down()
    {
        // Cannot reverse password hashing
        // Original passwords are lost (which is the point of hashing)
        throw new Exception('Cannot reverse password hashing migration');
    }
}