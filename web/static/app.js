/* =========================================================
   QuizHub Player - app.js
   No control buttons. Reacts to WebSocket events from admin.
   ========================================================= */
(function () {
  'use strict';

  let playerId = null;
  let playerNickname = '';
  let gameStatus = 'lobby'; // lobby, countdown, question, reveal, finished
  let currentQuestion = null;
  let questionIndex = 0;
  let totalQuestions = 0;
  let timeLeft = 0;
  let timeLimit = 15;
  let selectedAnswer = null;
  let correctAnswer = null; // revealed after timer
  let myResult = null; // {correct, score_earned, total_score}
  let myRank = null;
  let countdownLeft = 0;
  let timerInterval = null;
  let socket = null;
  let reconnectTimer = null;
  let answerSubmitted = false;

  const API = '';
  const $ = (sel) => document.querySelector(sel);

  function el(tag, attrs, ...children) {
    const node = document.createElement(tag);
    if (attrs) {
      Object.entries(attrs).forEach(([k, v]) => {
        if (k === 'className') node.className = v;
        else if (k.startsWith('data-')) node.setAttribute(k, v);
        else if (k === 'onclick') node.addEventListener('click', v);
        else if (k === 'disabled') node.disabled = v;
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
    const res = await fetch(API + path, { headers: { 'Content-Type': 'application/json' }, ...opts });
    const data = await res.json();
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  // ---- WebSocket ----
  function connectWS() {
    if (socket && socket.readyState <= 1) return;
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${location.host}/api/ws?role=player&player_id=${playerId || ''}`;
    try { socket = new WebSocket(url); } catch (_) { reconnectTimer = setTimeout(connectWS, 5000); return; }

    socket.onopen = () => clearTimeout(reconnectTimer);
    socket.onmessage = (evt) => { try { handleWS(JSON.parse(evt.data)); } catch (_) {} };
    socket.onclose = () => { reconnectTimer = setTimeout(connectWS, 5000); };
    socket.onerror = () => { try { socket.close(); } catch (_) {} };
  }

  function disconnectWS() {
    clearTimeout(reconnectTimer);
    if (socket) { try { socket.close(); } catch (_) {} socket = null; }
  }

  function handleWS(msg) {
    switch (msg.event) {
      case 'game_countdown':
        gameStatus = 'countdown';
        countdownLeft = msg.data.duration || 10;
        totalQuestions = msg.data.total_questions || 0;
        startCountdown();
        render();
        break;

      case 'new_question':
        gameStatus = 'question';
        currentQuestion = msg.data.current_question;
        questionIndex = msg.data.question_index || 0;
        totalQuestions = msg.data.total_questions || 0;
        timeLeft = msg.data.time_left || 15;
        timeLimit = timeLeft;
        selectedAnswer = null;
        correctAnswer = null;
        myResult = null;
        answerSubmitted = false;
        startQuestionTimer();
        render();
        break;

      case 'time_up':
        gameStatus = 'reveal';
        correctAnswer = msg.data.correct_answer;
        clearInterval(timerInterval);
        render();
        break;

      case 'your_result':
        myResult = msg.data;
        render();
        break;

      case 'game_finished':
        gameStatus = 'finished';
        clearInterval(timerInterval);
        fetchMyRank();
        render();
        break;

      case 'game_reset':
        resetAll();
        render();
        break;

      case 'player_kicked':
        resetAll();
        render();
        setTimeout(() => {
          const err = $('[data-testid="join-error"]');
          if (err) err.textContent = 'You were removed from the game';
        }, 100);
        break;

      case 'players_update':
        updatePlayerList(msg.data);
        break;

      case 'leaderboard_update':
        // Update rank if we're on finished screen
        if (gameStatus === 'finished' && msg.data) {
          const me = msg.data.find(e => e.player_id === playerId);
          if (me) myRank = me.rank;
          render();
        }
        break;
    }
  }

  function resetAll() {
    playerId = null;
    playerNickname = '';
    gameStatus = 'lobby';
    currentQuestion = null;
    selectedAnswer = null;
    correctAnswer = null;
    myResult = null;
    myRank = null;
    answerSubmitted = false;
    clearInterval(timerInterval);
    disconnectWS();
  }

  async function fetchMyRank() {
    try {
      const lb = await api('/api/leaderboard');
      const me = lb.find(e => e.player_id === playerId);
      if (me) { myRank = me.rank; render(); }
    } catch (_) {}
  }

  // ---- Timers ----
  function startCountdown() {
    clearInterval(timerInterval);
    timerInterval = setInterval(() => {
      countdownLeft -= 1;
      const cdEl = $('[data-testid="countdown-number"]');
      if (cdEl) cdEl.textContent = String(Math.max(0, countdownLeft));
      if (countdownLeft <= 0) clearInterval(timerInterval);
    }, 1000);
  }

  function startQuestionTimer() {
    clearInterval(timerInterval);
    timerInterval = setInterval(() => {
      timeLeft -= 1;
      const fill = $('[data-testid="timer-fill"]');
      if (fill) {
        const pct = Math.max(0, (timeLeft / timeLimit) * 100);
        fill.style.width = pct + '%';
        fill.classList.toggle('warning', timeLeft <= 5 && timeLeft > 2);
        fill.classList.toggle('critical', timeLeft <= 2);
      }
      const num = $('[data-testid="timer-number"]');
      if (num) num.textContent = Math.max(0, timeLeft) + 's';
      if (timeLeft <= 0) clearInterval(timerInterval);
    }, 1000);
  }

  // ---- Render ----
  function render() {
    const app = $('#app');
    app.innerHTML = '';

    app.appendChild(el('header', { className: 'header' },
      el('h1', null, 'QuizHub'),
      el('p', null, playerId ? `Playing as ${playerNickname}` : 'Real-time multiplayer trivia')
    ));

    if (!playerId) renderJoin(app);
    else if (gameStatus === 'lobby') renderLobby(app);
    else if (gameStatus === 'countdown') renderCountdown(app);
    else if (gameStatus === 'question' || gameStatus === 'reveal') renderQuestion(app);
    else if (gameStatus === 'finished') renderFinished(app);
  }

  function renderJoin(app) {
    const card = el('div', { className: 'card join-screen', 'data-testid': 'join-screen' },
      el('h2', null, 'Join the Quiz'),
      el('p', { className: 'subtitle' }, 'Enter your nickname to get started'),
      el('div', { className: 'input-group' },
        el('input', { id: 'nickname-input', type: 'text', placeholder: 'Your nickname...', 'data-testid': 'nickname-input', maxlength: '30' }),
        el('button', { className: 'btn btn-primary', 'data-testid': 'join-btn', onclick: handleJoin }, 'Join Game')
      ),
      el('p', { className: 'error-msg', 'data-testid': 'join-error' })
    );
    app.appendChild(card);
    setTimeout(() => {
      const inp = $('#nickname-input');
      if (inp) { inp.focus(); inp.addEventListener('keydown', (e) => { if (e.key === 'Enter') handleJoin(); }); }
    }, 50);
  }

  async function handleJoin() {
    const input = $('#nickname-input');
    const errorEl = $('[data-testid="join-error"]');
    const btn = $('[data-testid="join-btn"]');
    const nickname = input.value.trim();
    if (!nickname) { errorEl.textContent = 'Please enter a nickname'; return; }

    btn.disabled = true; btn.textContent = 'Joining...'; errorEl.textContent = '';
    try {
      const data = await api('/api/join', { method: 'POST', body: JSON.stringify({ nickname }) });
      playerId = data.player_id;
      playerNickname = nickname;
      connectWS();
      try {
        const s = await api('/api/game/state');
        if (s.status && s.status !== 'lobby') {
          gameStatus = s.status;
          if (s.current_question) currentQuestion = s.current_question;
          questionIndex = s.question_index || 0;
          totalQuestions = s.total_questions || 0;
          timeLeft = s.time_left || 0;
          timeLimit = timeLeft || 15;
          if (s.status === 'reveal' && s.correct_answer !== undefined) correctAnswer = s.correct_answer;
        }
      } catch (_) {}
      render();
    } catch (err) {
      errorEl.textContent = err.message || 'Failed to join';
      btn.disabled = false; btn.textContent = 'Join Game';
    }
  }

  function renderLobby(app) {
    const card = el('div', { className: 'card lobby-screen', 'data-testid': 'lobby-screen' },
      el('h2', null, 'Lobby'),
      el('p', { className: 'lobby-info' }, 'Waiting for the host to start the game...'),
      el('ul', { className: 'player-list', 'data-testid': 'player-list' }),
      el('div', { className: 'waiting' },
        el('div', { className: 'spinner' }),
      )
    );
    app.appendChild(card);
    api('/api/players').then(players => updatePlayerList(players)).catch(() => {});
  }

  function updatePlayerList(players) {
    const list = $('[data-testid="player-list"]');
    if (!list) return;
    list.innerHTML = '';
    (players || []).forEach(p => {
      const isYou = p.player_id === playerId;
      list.appendChild(el('li', { className: 'player-chip' + (isYou ? ' you' : ''), 'data-testid': 'player-chip' },
        p.nickname + (isYou ? ' (you)' : '')));
    });
  }

  function renderCountdown(app) {
    const card = el('div', { className: 'card countdown-screen', 'data-testid': 'countdown-screen' },
      el('h2', null, 'Get Ready!'),
      el('p', { className: 'subtitle' }, `${totalQuestions} questions coming up`),
      el('div', { className: 'countdown-circle' },
        el('span', { className: 'countdown-number', 'data-testid': 'countdown-number' }, String(Math.max(0, countdownLeft)))
      ),
    );
    app.appendChild(card);
  }

  function renderQuestion(app) {
    const q = currentQuestion;
    if (!q) { app.appendChild(el('div', { className: 'card waiting' }, el('div', { className: 'spinner' }), el('p', null, 'Loading...'))); return; }

    const optLabels = ['A', 'B', 'C', 'D', 'E', 'F'];
    const isReveal = gameStatus === 'reveal';

    const card = el('div', { className: 'card question-screen', 'data-testid': 'question-screen' },
      el('div', { className: 'question-meta' },
        el('span', { className: 'question-counter', 'data-testid': 'question-counter' }, `Question ${questionIndex + 1} of ${totalQuestions}`),
        el('span', { className: 'timer-number', 'data-testid': 'timer-number' }, isReveal ? 'Time\'s up!' : (timeLeft + 's')),
      ),
      el('div', { className: 'timer-bar' },
        el('div', { className: 'timer-fill' + (isReveal ? ' critical' : ''), id: 'timer-fill', 'data-testid': 'timer-fill',
          style: isReveal ? 'width:0%' : `width:${Math.max(0, (timeLeft / timeLimit) * 100)}%` })
      ),
      el('p', { className: 'question-text', 'data-testid': 'question-text' }, q.text),
      el('div', { className: 'options-grid', 'data-testid': 'options-grid' },
        ...q.options.map((opt, i) => {
          let cls = 'option-btn';
          if (selectedAnswer === i) cls += ' selected';
          if (isReveal) {
            if (i === correctAnswer) cls += ' correct';
            if (selectedAnswer === i && i !== correctAnswer) cls += ' wrong';
            cls += ' disabled';
          }
          if (answerSubmitted && !isReveal) cls += ' disabled';

          return el('button', {
            className: cls,
            'data-testid': `option-${i}`,
            onclick: () => handleAnswer(i),
            disabled: answerSubmitted || isReveal,
          }, el('span', { className: 'option-label' }, optLabels[i] || String(i)), opt);
        })
      ),
    );

    // Show feedback
    if (answerSubmitted && !isReveal) {
      card.appendChild(el('div', { className: 'answer-locked', 'data-testid': 'answer-locked' }, 'Answer locked! Waiting for timer...'));
    }

    if (isReveal && myResult) {
      const isCorrect = myResult.correct;
      card.appendChild(el('div', { className: 'result-toast ' + (isCorrect ? 'correct' : 'wrong'), 'data-testid': 'result-toast' },
        el('span', null, isCorrect ? 'Correct!' : (selectedAnswer == null ? 'No answer!' : 'Wrong!')),
        el('span', { className: 'result-score' }, isCorrect ? `+${myResult.score_earned || 0}` : '+0'),
      ));
    } else if (isReveal && !myResult && selectedAnswer == null) {
      card.appendChild(el('div', { className: 'result-toast wrong', 'data-testid': 'result-toast' },
        el('span', null, 'Time\'s up! No answer submitted'),
      ));
    }

    if (isReveal) {
      card.appendChild(el('div', { className: 'waiting-next', 'data-testid': 'waiting-next' },
        el('div', { className: 'spinner' }),
        el('p', null, 'Waiting for host to advance...'),
      ));
    }

    app.appendChild(card);
  }

  async function handleAnswer(index) {
    if (answerSubmitted || gameStatus !== 'question') return;
    selectedAnswer = index;
    answerSubmitted = true;
    render();

    try {
      await api('/api/answer', {
        method: 'POST',
        body: JSON.stringify({ player_id: playerId, question_id: currentQuestion.id, answer: index }),
      });
    } catch (err) {
      if (err.message && err.message.includes('player not found')) {
        resetAll(); render(); return;
      }
    }
  }

  function renderFinished(app) {
    const card = el('div', { className: 'card finished-screen', 'data-testid': 'finished-screen' },
      el('h2', null, 'Game Over!'),
      myRank
        ? el('div', { className: 'rank-display', 'data-testid': 'rank-display' },
            el('p', { className: 'rank-label' }, 'Your Rank'),
            el('div', { className: 'rank-number' + (myRank <= 3 ? ' top' : '') }, `#${myRank}`),
            myResult ? el('p', { className: 'total-score' }, `Total: ${myResult.total_score || 0} pts`) : null,
          )
        : el('div', { className: 'waiting' }, el('div', { className: 'spinner' }), el('p', null, 'Loading your rank...')),
    );
    app.appendChild(card);
  }

  // ---- Init ----
  document.addEventListener('DOMContentLoaded', render);
})();
