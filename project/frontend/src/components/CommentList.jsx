import { useState, useEffect } from 'react';
import { getComments, createComment, deleteComment } from '../services/api';

function CommentList({ articleId }) {
  const [comments, setComments] = useState([]);
  const [newComment, setNewComment] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchComments();
  }, [articleId]);

  const fetchComments = async () => {
    try {
      const response = await getComments(articleId);
      setComments(response.data);
    } catch (error) {
      console.error('Error fetching comments:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    
    if (!newComment.trim()) {
      return;
    }

    try {
      await createComment({
        article_id: articleId,
        user_id: 1, // Mock user ID
        content: newComment,
      });
      
      setNewComment('');
      fetchComments(); // Refresh comments
    } catch (error) {
      alert('Erreur lors de l\'ajout du commentaire');
      console.error('Error creating comment:', error);
    }
  };

  const handleDelete = async (commentId) => {
    try {
      await deleteComment(commentId);
      fetchComments(); // Refresh comments
    } catch (error) {
      alert('Erreur lors de la suppression du commentaire: ' + error.message);
      console.error('Error deleting comment:', error);
    }
  };

  if (loading) {
    return <div>Chargement des commentaires...</div>;
  }

  return (
    <div>
      <h4 style={{ marginBottom: '1rem' }}>Commentaires</h4>
      
      <div style={{ marginBottom: '1rem' }}>
        {comments.length === 0 ? (
          <p style={{ color: '#7f8c8d', fontStyle: 'italic' }}>Aucun commentaire pour le moment</p>
        ) : (
          comments.map(comment => (
            <div 
              key={comment.id} 
              style={{ 
                padding: '0.8rem',
                marginBottom: '0.5rem',
                backgroundColor: '#f8f9fa',
                borderRadius: '4px',
                position: 'relative'
              }}
            >
              <p style={{ marginBottom: '0.5rem' }}>
                 {comment.content}
              </p>

              
              <div style={{ fontSize: '0.85em', color: '#7f8c8d' }}>
                — {comment.user?.name || 'Utilisateur'}
              </div>

              <button
                onClick={() => handleDelete(comment.id)}
                style={{
                  position: 'absolute',
                  top: '0.5rem',
                  right: '0.5rem',
                  fontSize: '0.8em',
                  padding: '0.3em 0.6em',
                  backgroundColor: '#e74c3c'
                }}
              >
                ✕
              </button>
            </div>
          ))
        )}
      </div>

      <form onSubmit={handleSubmit}>
        <div style={{ marginBottom: '0.5rem' }}>
          <textarea
            value={newComment}
            onChange={(e) => setNewComment(e.target.value)}
            placeholder="Ajouter un commentaire..."
            rows={3}
            style={{ width: '100%' }}
          />
        </div>
        <button type="submit" disabled={!newComment.trim()}>
          Publier
        </button>
      </form>
    </div>
  );
}

export default CommentList;

