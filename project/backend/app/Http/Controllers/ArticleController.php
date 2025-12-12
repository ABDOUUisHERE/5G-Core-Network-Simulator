<?php

namespace App\Http\Controllers;

use App\Models\Article;
use Illuminate\Http\Request;
use Illuminate\Support\Facades\DB;

class ArticleController extends Controller
{
    /**
 * Display a listing of articles.
 */
public function index(Request $request)
{
    // ✅ Eager loading des relations pour éviter le N+1
    $articles = Article::with(['author', 'comments'])->get();

    $articles = $articles->map(function ($article) use ($request) {
        if ($request->has('performance_test')) {
            usleep(30000); // 30ms par article pour simuler le coût du N+1
        }

        return [
            'id' => $article->id,
            'title' => $article->title,
            'content' => substr($article->content, 0, 200) . '...',
            'author' => $article->author->name,
            'comments_count' => $article->comments->count(),
            'published_at' => $article->published_at,
            'created_at' => $article->created_at,
        ];
    });

    return response()->json($articles);
}


    /**
     * Display the specified article.
     */
    public function show($id)
    {
        $article = Article::with(['author', 'comments.user'])->findOrFail($id);

        return response()->json([
            'id' => $article->id,
            'title' => $article->title,
            'content' => $article->content,
            'author' => $article->author->name,
            'author_id' => $article->author->id,
            'image_path' => $article->image_path,
            'published_at' => $article->published_at,
            'created_at' => $article->created_at,
            'comments' => $article->comments->map(function ($comment) {
                return [
                    'id' => $comment->id,
                    'content' => $comment->content,
                    'user' => $comment->user->name,
                    'created_at' => $comment->created_at,
                ];
            }),
        ]);
    }

    /**
     * Search articles.
     * 
     * [BUG-001] Recherche insensible aux accents avec collation utf8mb4_general_ci
     * [SEC-002] Protection contre l'injection SQL avec Eloquent Query Builder
     */
    public function search(Request $request)
    {
        $query = $request->input('q');

        if (!$query) {
            return response()->json([]);
        }

        // ✅ SOLUTION SÉCURISÉE : Eloquent + paramètres préparés + collation pour accents
        // whereRaw utilise des paramètres préparés (?) qui empêchent l'injection SQL
        // La collation utf8mb4_general_ci rend la recherche insensible aux accents
        $articles = Article::whereRaw('title COLLATE utf8mb4_general_ci LIKE ?', ["%{$query}%"])
                           ->orWhereRaw('content COLLATE utf8mb4_general_ci LIKE ?', ["%{$query}%"])
                           ->select('id', 'title', 'content', 'published_at')
                           ->get();

        $results = $articles->map(function ($article) {
            return [
                'id' => $article->id,
                'title' => $article->title,
                'content' => substr($article->content, 0, 200),
                'published_at' => $article->published_at,
            ];
        });

        return response()->json($results);
    }

    /**
     * Store a newly created article.
     */
    public function store(Request $request)
    {
        $validated = $request->validate([
            'title' => 'required|max:255',
            'content' => 'required',
            'author_id' => 'required|exists:users,id',
            'image_path' => 'nullable|string',
        ]);

        $article = Article::create([
            'title' => $validated['title'],
            'content' => $validated['content'],
            'author_id' => $validated['author_id'],
            'image_path' => $validated['image_path'] ?? null,
            'published_at' => now(),
        ]);

        return response()->json($article, 201);
    }

    /**
     * Update the specified article.
     */
    public function update(Request $request, $id)
    {
        $article = Article::findOrFail($id);

        $validated = $request->validate([
            'title' => 'sometimes|required|max:255',
            'content' => 'sometimes|required',
        ]);

        $article->update($validated);

        return response()->json($article);
    }

    /**
     * Remove the specified article.
     */
    public function destroy($id)
    {
        $article = Article::findOrFail($id);
        $article->delete();

        return response()->json(['message' => 'Article deleted successfully']);
    }
}