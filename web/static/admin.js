/* =========================================================
   QuizHub Admin Panel - admin.js
   ========================================================= */

(function () {
  'use strict';

  let adminToken = null;
  let socket = null;
  let reconnectTimer = null;
  let gameStatus = 'lobby';
  let players = [];
  let leaderboard = [];
  let questions = [];
  let categories = [];
  let answerStats = { total: 0, correct: 0, wrong: 0 };
  let currentQuestion = null;
  let questionIndex = 0;
  let totalQuestions = 0;
  let editingQuestion = null;

  const API = '';
  const $ = (sel) => document.querySelector(sel);

  function el(tag, attrs, ...children) {
    const node = document.createElement(tag);
    if (attrs) {
      Object.entries(attrs).forEach(([k, v]) => {
        if (k === 'className') node.className = v;
        else if (k.startsWith('data-')) node.setAttribute(k, v);
        else if (k === 'onclick') node.addEventListener('click', v);
        else if (k === 'onchange') node.addEventListener('change', v);
        else if (k === 'onkeydown') node.addEventListener('keydown', v);
        else if (k === 'disabled') node.disabled = v;
        else if (k === 'selected') node.selected = v;
        else if (k === 'checked') node.checked = v;
        else node.setAttribute(k, v);
      });
    }
    children.flat().forEach(c => {
      if (c == null) return;
      node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return node;
  }

  async function api(path, opts = {}) {
    const headers = { 'Content-Type': 'application/json' };
    if (adminToken) headers['X-Admin-Token'] = adminToken;
    const res = await fetch(API + path, { headers, ...opts });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  // ---- WebSocket ----
  function connectWS() {
    if (socket && socket.readyState <= 1) return;
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${location.host}/api/ws?role=admin`;
    socket = new WebSocket(url);

    socket.onopen = () => {
      clearTimeout(reconnectTimer);
      updateConnectionStatus(true);
    };

    socket.onmessage = (evt) => {
      try {
        const msg = JSON.parse(evt.data);
        handleWSMessage(msg);
      } catch (_) {}
    };

    socket.onclose = () => {
      updateConnectionStatus(false);
      reconnectTimer = setTimeout(connectWS, 3000);
    };

    socket.onerror = () => socket.close();
  }

  function handleWSMessage(msg) {
    switch (msg.event) {
      case 'player_joined':
      case 'players_update':
        if (Array.isArray(msg.data)) {
          players = msg.data;
        }
        refreshPlayers();
        break;

      case 'game_started':
      case 'new_question':
        gameStatus = msg.data.status || 'question';
        currentQuestion = msg.data.current_question;
        questionIndex = msg.data.question_index || 0;
        totalQuestions = msg.data.total_questions || 0;
        answerStats = { total: 0, correct: 0, wrong: 0 };
        refreshGameControls();
        refreshAnswerStats();
        break;

      case 'game_finished':
        gameStatus = 'finished';
        currentQuestion = null;
        refreshGameControls();
        break;

      case 'game_reset':
        gameStatus = 'lobby';
        currentQuestion = null;
        answerStats = { total: 0, correct: 0, wrong: 0 };
        players = [];
        leaderboard = [];
        refreshAll();
        break;

      case 'player_answered':
        answerStats.total = msg.data.total_answers || 0;
        answerStats.correct = msg.data.correct_count || 0;
        answerStats.wrong = msg.data.wrong_count || 0;
        refreshAnswerStats();
        break;

      case 'leaderboard_update':
        leaderboard = msg.data || [];
        refreshLeaderboard();
        break;
    }
  }

  function updateConnectionStatus(connected) {
    const dot = $('[data-testid="ws-status-dot"]');
    const text = $('[data-testid="ws-status-text"]');
    if (dot) {
      dot.className = 'status-dot ' + (connected ? 'connected' : 'disconnected');
    }
    if (text) {
      text.textContent = connected ? 'Connected' : 'Reconnecting...';
    }
  }

  // ---- Render ----
  function renderApp() {
    const app = $('#admin-app');
    app.innerHTML = '';

    if (!adminToken) {
      renderPINScreen(app);
    } else {
      renderDashboard(app);
    }
  }

  function renderPINScreen(app) {
    const card = el('div', { className: 'card pin-screen', 'data-testid': 'pin-screen' },
      el('h2', null, 'Admin Access'),
      el('p', { className: 'subtitle' }, 'Enter the admin PIN to continue'),
      el('input', {
        id: 'pin-input',
        type: 'password',
        placeholder: 'Enter PIN...',
        'data-testid': 'pin-input',
        maxlength: '20',
        onkeydown: (e) => { if (e.key === 'Enter') handlePINSubmit(); },
      }),
      el('div', { style: 'margin-top:1rem' },
        el('button', {
          className: 'btn btn-primary',
          'data-testid': 'pin-submit-btn',
          onclick: handlePINSubmit,
          style: 'width:100%',
        }, 'Unlock')
      ),
      el('p', { className: 'error-msg', 'data-testid': 'pin-error' })
    );
    app.appendChild(card);
    setTimeout(() => { const inp = $('#pin-input'); if (inp) inp.focus(); }, 50);
  }

  async function handlePINSubmit() {
    const input = $('#pin-input');
    const errorEl = $('[data-testid="pin-error"]');
    const pin = input.value.trim();

    if (!pin) { errorEl.textContent = 'PIN is required'; return; }

    try {
      const data = await api('/api/admin/auth', {
        method: 'POST',
        body: JSON.stringify({ pin }),
      });
      adminToken = data.token;
      connectWS();
      await loadInitialData();
      renderApp();
    } catch (err) {
      errorEl.textContent = err.message || 'Invalid PIN';
    }
  }

  async function loadInitialData() {
    try {
      const [p, q, s, c] = await Promise.all([
        api('/api/players'),
        api('/api/questions'),
        api('/api/game/state'),
        api('/api/categories'),
      ]);
      players = p || [];
      questions = q || [];
      categories = c || [];
      gameStatus = s.status || 'lobby';
      currentQuestion = s.current_question;
      questionIndex = s.question_index || 0;
      totalQuestions = s.total_questions || 0;
    } catch (_) {}

    try {
      leaderboard = await api('/api/leaderboard');
    } catch (_) { leaderboard = []; }
  }

  function renderDashboard(app) {
    // Header
    const header = el('div', { className: 'admin-header' },
      el('div', null,
        el('h1', null, 'QuizHub Admin'),
        el('p', { className: 'subtitle' },
          el('span', { className: 'status-dot connected', 'data-testid': 'ws-status-dot' }),
          el('span', { 'data-testid': 'ws-status-text' }, 'Connected'),
        ),
      ),
      el('a', { href: '/', className: 'btn btn-secondary btn-sm', 'data-testid': 'back-to-game-btn' }, 'Player View')
    );
    app.appendChild(header);

    // Grid
    const grid = el('div', { className: 'admin-grid' });

    // Game Controls Card
    grid.appendChild(renderGameControlsCard());

    // Answer Stats Card
    grid.appendChild(renderAnswerStatsCard());

    // Players Card
    grid.appendChild(renderPlayersCard());

    // Leaderboard Card
    grid.appendChild(renderLeaderboardCard());

    // Questions Card (full width)
    grid.appendChild(renderQuestionsCard());

    app.appendChild(grid);
  }

  // ---- Game Controls ----
  function renderGameControlsCard() {
    const statusClass = gameStatus === 'question' ? 'question' : gameStatus === 'finished' ? 'finished' : 'lobby';

    const card = el('div', { className: 'card', 'data-testid': 'game-controls-card' },
      el('h3', null,
        'Game Controls',
        el('span', { className: `game-status ${statusClass}`, 'data-testid': 'game-status-badge' }, gameStatus.toUpperCase())
      ),
    );

    if (currentQuestion) {
      card.appendChild(el('p', { style: 'color:var(--text-secondary);font-size:0.85rem;margin-bottom:0.5rem' },
        `Q${questionIndex + 1}/${totalQuestions}: ${currentQuestion.text}`
      ));
    }

    // Timer config
    const timerRow = el('div', { className: 'timer-config' },
      el('label', null, 'Timer:'),
      el('input', {
        type: 'number',
        id: 'timer-input',
        value: '15',
        'data-testid': 'timer-input',
      }),
      el('label', null, 'sec'),
      el('button', {
        className: 'btn btn-secondary btn-sm',
        'data-testid': 'set-timer-btn',
        onclick: handleSetTimer,
      }, 'Set')
    );
    card.appendChild(timerRow);

    // Action buttons
    const actions = el('div', { className: 'control-row' });

    if (gameStatus === 'lobby') {
      actions.appendChild(el('button', {
        className: 'btn btn-accent',
        'data-testid': 'admin-start-btn',
        onclick: handleAdminStart,
      }, 'Start Game'));
    } else if (gameStatus === 'question') {
      actions.appendChild(el('button', {
        className: 'btn btn-accent',
        'data-testid': 'admin-next-btn',
        onclick: handleAdminNext,
      }, 'Next Question'));
    }

    if (gameStatus !== 'lobby') {
      actions.appendChild(el('button', {
        className: 'btn btn-danger btn-sm',
        'data-testid': 'admin-reset-btn',
        onclick: handleAdminReset,
      }, 'Reset Game'));
    }

    card.appendChild(actions);
    return card;
  }

  async function handleAdminStart() {
    try { await api('/api/game/start', { method: 'POST' }); } catch (_) {}
  }

  async function handleAdminNext() {
    try { await api('/api/game/next', { method: 'POST' }); } catch (_) {}
  }

  async function handleAdminReset() {
    if (!confirm('Reset the game? This clears all players and scores.')) return;
    try { await api('/api/game/reset', { method: 'POST' }); } catch (_) {}
  }

  async function handleSetTimer() {
    const input = $('#timer-input');
    const val = parseInt(input.value);
    if (isNaN(val) || val < 5 || val > 120) return;
    try { await api('/api/admin/timer', { method: 'POST', body: JSON.stringify({ time_limit: val }) }); } catch (_) {}
  }

  // ---- Answer Stats ----
  function renderAnswerStatsCard() {
    const card = el('div', { className: 'card', 'data-testid': 'answer-stats-card' },
      el('h3', null, 'Live Answer Stats'),
      el('div', { className: 'answer-stats', 'data-testid': 'answer-stats' },
        el('div', { className: 'stat-box' },
          el('div', { className: 'stat-value', 'data-testid': 'stat-total' }, String(answerStats.total)),
          el('div', { className: 'stat-label' }, 'Answers'),
        ),
        el('div', { className: 'stat-box correct-stat' },
          el('div', { className: 'stat-value', 'data-testid': 'stat-correct' }, String(answerStats.correct)),
          el('div', { className: 'stat-label' }, 'Correct'),
        ),
        el('div', { className: 'stat-box wrong-stat' },
          el('div', { className: 'stat-value', 'data-testid': 'stat-wrong' }, String(answerStats.wrong)),
          el('div', { className: 'stat-label' }, 'Wrong'),
        ),
      ),
    );
    return card;
  }

  // ---- Players ----
  function renderPlayersCard() {
    const card = el('div', { className: 'card', 'data-testid': 'players-card' },
      el('h3', null, 'Players', el('span', { className: 'badge', 'data-testid': 'player-count-badge' }, String(players.length))),
    );

    if (players.length === 0) {
      card.appendChild(el('div', { className: 'empty-state' }, 'No players yet'));
    } else {
      const table = el('table', { className: 'player-table' },
        el('thead', null,
          el('tr', null,
            el('th', null, 'Nickname'),
            el('th', null, 'Score'),
            el('th', null, ''),
          )
        ),
        el('tbody', { 'data-testid': 'players-tbody' },
          ...players.map(p =>
            el('tr', { 'data-testid': `player-row-${p.player_id}` },
              el('td', null, p.nickname),
              el('td', null, String(p.score)),
              el('td', null,
                el('button', {
                  className: 'kick-btn',
                  'data-testid': `kick-${p.player_id}`,
                  onclick: () => handleKick(p.player_id, p.nickname),
                }, 'Kick')
              ),
            )
          )
        )
      );
      card.appendChild(table);
    }
    return card;
  }

  async function handleKick(playerID, nickname) {
    if (!confirm(`Kick ${nickname}?`)) return;
    try {
      await api('/api/admin/kick', {
        method: 'POST',
        body: JSON.stringify({ player_id: playerID }),
      });
      players = players.filter(p => p.player_id !== playerID);
      refreshPlayers();
    } catch (_) {}
  }

  // ---- Leaderboard ----
  function renderLeaderboardCard() {
    const card = el('div', { className: 'card', 'data-testid': 'leaderboard-card' },
      el('h3', null, 'Leaderboard'),
    );

    const list = el('div', { 'data-testid': 'admin-leaderboard' });
    if (leaderboard.length === 0) {
      list.appendChild(el('div', { className: 'empty-state' }, 'No scores yet'));
    } else {
      leaderboard.forEach((e, i) => {
        let rankCls = 'lb-rank';
        if (i === 0) rankCls += ' gold';
        else if (i === 1) rankCls += ' silver';
        else if (i === 2) rankCls += ' bronze';

        list.appendChild(el('div', { className: 'lb-entry' },
          el('span', { className: rankCls }, `#${e.rank}`),
          el('span', { className: 'lb-name' }, e.nickname),
          el('span', { className: 'lb-score' }, String(e.score)),
        ));
      });
    }
    card.appendChild(list);
    return card;
  }

  // ---- Questions ----
  function renderQuestionsCard() {
    const card = el('div', { className: 'card full-width', 'data-testid': 'questions-card' },
      el('h3', null,
        'Question Bank',
        el('span', { className: 'badge' }, String(questions.length)),
      ),
      el('div', { className: 'btn-group', style: 'margin-bottom:1rem' },
        el('button', {
          className: 'btn btn-primary btn-sm',
          'data-testid': 'add-question-btn',
          onclick: () => showAddQuestionModal(),
        }, '+ Add Question'),
        el('button', {
          className: 'btn btn-secondary btn-sm',
          'data-testid': 'refresh-questions-btn',
          onclick: refreshQuestions,
        }, 'Refresh'),
      ),
    );

    const scroll = el('div', { className: 'question-scroll', 'data-testid': 'question-list' });

    questions.forEach(q => {
      const labels = ['A', 'B', 'C', 'D', 'E', 'F'];
      const correctLabel = labels[q.answer] || '?';

      scroll.appendChild(el('div', { className: 'question-item', 'data-testid': `question-${q.id}` },
        el('div', { className: 'q-header' },
          el('div', null,
            el('div', { className: 'q-text' }, q.text),
            el('div', { className: 'q-meta' },
              el('span', { className: 'q-category' }, q.category),
              el('span', { className: 'q-category', style: 'color:var(--accent)' }, `Answer: ${correctLabel}`),
            ),
          ),
          el('div', { className: 'q-actions' },
            el('button', {
              className: 'btn btn-secondary btn-sm',
              'data-testid': `edit-q-${q.id}`,
              onclick: () => showEditQuestionModal(q),
            }, 'Edit'),
            el('button', {
              className: 'btn btn-danger btn-sm',
              'data-testid': `delete-q-${q.id}`,
              onclick: () => handleDeleteQuestion(q.id),
            }, 'Delete'),
          ),
        ),
      ));
    });

    card.appendChild(scroll);
    return card;
  }

  async function handleDeleteQuestion(id) {
    if (!confirm('Delete this question?')) return;
    try {
      await api('/api/questions/delete', {
        method: 'POST',
        body: JSON.stringify({ id }),
      });
      questions = questions.filter(q => q.id !== id);
      refreshQuestions();
    } catch (_) {}
  }

  async function refreshQuestions() {
    try {
      questions = await api('/api/questions');
      categories = await api('/api/categories');
    } catch (_) {}
    const card = $('[data-testid="questions-card"]');
    if (card) {
      card.replaceWith(renderQuestionsCard());
    }
  }

  // ---- Add / Edit Question Modal ----
  function showAddQuestionModal() {
    editingQuestion = null;
    showQuestionModal({ text: '', options: ['', '', '', ''], answer: 0, category: '' });
  }

  function showEditQuestionModal(q) {
    editingQuestion = q;
    showQuestionModal({ ...q, options: [...q.options] });
  }

  function showQuestionModal(data) {
    const existing = $('[data-testid="question-modal"]');
    if (existing) existing.remove();

    const labels = ['A', 'B', 'C', 'D'];

    const overlay = el('div', { className: 'modal-overlay', 'data-testid': 'question-modal',
      onclick: (e) => { if (e.target === overlay) overlay.remove(); }
    },
      el('div', { className: 'modal-content' },
        el('h3', null, editingQuestion ? 'Edit Question' : 'Add Question'),
        el('div', { className: 'add-question-form' },
          el('input', {
            id: 'q-text',
            type: 'text',
            placeholder: 'Question text...',
            value: data.text,
            'data-testid': 'modal-q-text',
          }),
          el('div', { className: 'options-inputs' },
            ...data.options.map((opt, i) =>
              el('div', { className: 'option-input-wrap' },
                el('span', { className: 'opt-label' }, labels[i] || String(i)),
                el('input', {
                  id: `q-opt-${i}`,
                  type: 'text',
                  placeholder: `Option ${labels[i]}`,
                  value: opt,
                  'data-testid': `modal-q-opt-${i}`,
                }),
              )
            )
          ),
          el('div', { className: 'form-row' },
            el('select', { id: 'q-answer', 'data-testid': 'modal-q-answer' },
              ...labels.map((l, i) =>
                el('option', { value: String(i), selected: i === data.answer }, `Correct: ${l}`)
              )
            ),
            el('input', {
              id: 'q-category',
              type: 'text',
              placeholder: 'Category',
              value: data.category || '',
              'data-testid': 'modal-q-category',
            }),
          ),
          el('div', { className: 'btn-group' },
            el('button', {
              className: 'btn btn-primary',
              'data-testid': 'modal-save-btn',
              onclick: handleSaveQuestion,
            }, editingQuestion ? 'Save Changes' : 'Add Question'),
            el('button', {
              className: 'btn btn-secondary',
              onclick: () => overlay.remove(),
            }, 'Cancel'),
          ),
        ),
      )
    );

    document.body.appendChild(overlay);
  }

  async function handleSaveQuestion() {
    const text = $('#q-text').value.trim();
    const options = [0, 1, 2, 3].map(i => $(`#q-opt-${i}`).value.trim());
    const answer = parseInt($('#q-answer').value);
    const category = $('#q-category').value.trim() || 'general';

    if (!text || options.some(o => !o)) return;

    const overlay = $('[data-testid="question-modal"]');

    try {
      if (editingQuestion) {
        await api('/api/questions/edit', {
          method: 'POST',
          body: JSON.stringify({ id: editingQuestion.id, text, options, answer, category }),
        });
      } else {
        await api('/api/questions/add', {
          method: 'POST',
          body: JSON.stringify({ text, options, answer, category }),
        });
      }
      if (overlay) overlay.remove();
      await refreshQuestions();
    } catch (err) {
      alert(err.message);
    }
  }

  // ---- Refresh helpers ----
  function refreshAll() {
    renderApp();
  }

  function refreshPlayers() {
    const card = $('[data-testid="players-card"]');
    if (card) card.replaceWith(renderPlayersCard());
    const countBadge = $('[data-testid="player-count-badge"]');
    if (countBadge) countBadge.textContent = String(players.length);
  }

  function refreshGameControls() {
    const card = $('[data-testid="game-controls-card"]');
    if (card) card.replaceWith(renderGameControlsCard());
  }

  function refreshAnswerStats() {
    const total = $('[data-testid="stat-total"]');
    const correct = $('[data-testid="stat-correct"]');
    const wrong = $('[data-testid="stat-wrong"]');
    if (total) total.textContent = String(answerStats.total);
    if (correct) correct.textContent = String(answerStats.correct);
    if (wrong) wrong.textContent = String(answerStats.wrong);
  }

  function refreshLeaderboard() {
    const card = $('[data-testid="leaderboard-card"]');
    if (card) card.replaceWith(renderLeaderboardCard());
  }

  // ---- Init ----
  document.addEventListener('DOMContentLoaded', renderApp);
})();
