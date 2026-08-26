/*
   QuizHub Player - app.js
   Live multiplayer quiz participant experience.
*/
(function () {
  'use strict';

  let playerId = null;
  let playerNickname = '';
  let gameStatus = 'join'; // join, lobby, countdown, question, reveal, finished
  let currentQuestion = null;
  let questionIndex = 0;
  let totalQuestions = 0;
  let timeLeft = 0;
  let timeLimit = 15;
  let selectedAnswer = null;
  let correctAnswer = null;
  let myResult = null;
  let myRank = null;
  let countdownLeft = 0;
  let timerInterval = null;
  let socket = null;
  let reconnectTimer = null;
  let answerSubmitted = false;
  let roomCode = '';
  let joinInFlight = false;
  let toastTimer = null;

  const API = '';
  const $ = (sel) => document.querySelector(sel);

  function el(tag, attrs, ...children) {
    const node = document.createElement(tag);
    if (attrs) {
      Object.entries(attrs).forEach(([k, v]) => {
        if (k === 'className') node.className = v;
        else if (k === 'textContent') node.textContent = v;
        else if (k.startsWith('data-')) node.setAttribute(k, v);
        else if (k === 'onclick') node.addEventListener('click', v);
        else if (k === 'disabled') node.disabled = v;
        else if (k === 'aria-live') node.setAttribute('aria-live', v);
        else if (k === 'htmlFor') node.htmlFor = v;
        else node.setAttribute(k, v);
      });
    }
    children.flat().forEach((child) => {
      if (child == null) return;
      node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
    });
    return node;
  }

  function notify(message, tone = 'neutral') {
    let toast = $('[data-testid="player-toast"]');
    if (!toast) {
      toast = el('div', { className: 'toast', 'data-testid': 'player-toast', role: 'status', 'aria-live': 'polite' });
      document.body.appendChild(toast);
    }
    toast.className = `toast ${tone}`;
    toast.textContent = message;
    toast.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => { toast.hidden = true; }, 3600);
  }

  async function api(path, opts = {}) {
    const res = await fetch(API + path, {
      headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
      ...opts,
    });
    const raw = await res.text();
    let data = {};
    try { data = raw ? JSON.parse(raw) : {}; } catch (_) { data = { error: raw || 'Request failed' }; }
    if (!res.ok) throw new Error(data.error || 'Request failed');
    return data;
  }

  // ---- WebSocket ----
  function connectWS() {
    if (!playerId || (socket && socket.readyState <= 1)) return;
    clearTimeout(reconnectTimer);
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const url = `${proto}//${location.host}/api/ws?role=player&player_id=${encodeURIComponent(playerId)}`;
    try { socket = new WebSocket(url); } catch (_) { scheduleReconnect(); return; }
    socket.onopen = () => clearTimeout(reconnectTimer);
    socket.onmessage = (evt) => { try { handleWS(JSON.parse(evt.data)); } catch (_) {} };
    socket.onclose = () => { socket = null; scheduleReconnect(); };
    socket.onerror = () => { try { socket.close(); } catch (_) {} };
  }

  function scheduleReconnect() {
    if (!playerId || reconnectTimer) return;
    reconnectTimer = setTimeout(() => { reconnectTimer = null; connectWS(); }, 5000);
  }

  function disconnectWS() {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
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
        notify('You were removed from the game.', 'error');
        break;
      case 'players_update':
        updatePlayerList(msg.data);
        break;
      case 'leaderboard_update':
        if (gameStatus === 'finished' && msg.data) {
          const me = msg.data.find((entry) => entry.player_id === playerId);
          if (me) { myRank = me.rank; myResult = { ...myResult, total_score: me.score }; render(); }
        }
        break;
    }
  }

  function resetAll() {
    playerId = null;
    playerNickname = '';
    roomCode = '';
    gameStatus = 'join';
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
      const me = lb.find((entry) => entry.player_id === playerId);
      if (me) { myRank = me.rank; myResult = { ...myResult, total_score: me.score }; render(); }
    } catch (_) {}
  }

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
        fill.style.width = `${pct}%`;
        fill.classList.toggle('warning', timeLeft <= 5 && timeLeft > 2);
        fill.classList.toggle('critical', timeLeft <= 2);
      }
      const num = $('[data-testid="timer-number"]');
      if (num) {
        num.textContent = `${Math.max(0, timeLeft)}s`;
        num.setAttribute('aria-valuenow', String(Math.max(0, timeLeft)));
        num.setAttribute('aria-label', `${Math.max(0, timeLeft)} seconds remaining`);
      }
      if (timeLeft <= 0) clearInterval(timerInterval);
    }, 1000);
  }

  // ---- Render ----
  function render() {
    const app = $('#app');
    app.innerHTML = '';
    app.appendChild(el('header', { className: 'header' },
      el('div', { className: 'brand-lockup' },
        el('div', { className: 'brand-mark', 'aria-hidden': 'true' }, 'Q'),
        el('div', null,
          el('h1', null, 'QuizHub'),
          el('p', null, playerId ? `Playing as ${playerNickname}` : 'Real-time multiplayer trivia'))),
      playerId
        ? el('div', { className: 'session-pill' },
            el('span', { className: 'live-dot', 'aria-hidden': 'true' }),
            el('span', null, `Room ${roomCode}`))
        : el('a', { className: 'header-link', href: '/admin.html' }, 'Host a quiz')));

    if (gameStatus === 'join') renderJoin(app);
    else if (gameStatus === 'lobby') renderLobby(app);
    else if (gameStatus === 'countdown') renderCountdown(app);
    else if (gameStatus === 'question' || gameStatus === 'reveal') renderQuestion(app);
    else if (gameStatus === 'finished') renderFinished(app);
  }

  function renderJoin(app) {
    const pathMatch = location.pathname.match(/^\/join\/([^/]+)\/?$/i);
    const pathRoom = pathMatch ? decodeURIComponent(pathMatch[1]) : '';
    const urlRoom = (new URLSearchParams(location.search).get('room') || pathRoom).trim().toUpperCase();
    const card = el('section', { className: 'card join-screen', 'data-testid': 'join-screen' },
      el('div', { className: 'eyebrow' }, 'PLAYER ENTRY'),
      el('h2', null, 'Join the arena'),
      el('p', { className: 'subtitle' }, 'Enter the room code from your host. No account needed.'),
      el('div', { className: 'join-fields' },
        el('label', { htmlFor: 'room-input' }, 'Room code', el('span', { className: 'label-hint' }, '6 characters')),
        el('input', { id: 'room-input', type: 'text', placeholder: 'e.g. A3X7K2', 'data-testid': 'room-input', maxlength: '10', autocomplete: 'off', autocapitalize: 'characters', value: urlRoom, 'aria-describedby': 'join-helper' }),
        el('label', { htmlFor: 'nickname-input' }, 'Your display name'),
        el('input', { id: 'nickname-input', type: 'text', placeholder: 'What should we call you?', 'data-testid': 'nickname-input', maxlength: '30', autocomplete: 'nickname' }),
        el('button', { className: 'btn btn-primary btn-large', 'data-testid': 'join-btn', onclick: handleJoin, disabled: false }, 'Join room', el('span', { className: 'btn-arrow', 'aria-hidden': 'true' }, '→')),
      ),
      el('p', { id: 'join-helper', className: 'helper-text' }, 'Your host controls the pace. You answer, climb the leaderboard, and have fun.'),
      el('p', { className: 'error-msg', 'data-testid': 'join-error', role: 'alert' })
    );
    app.appendChild(card);
    app.appendChild(el('div', { className: 'feature-strip' },
      el('div', null, el('strong', null, 'LIVE'), el('span', null, 'Answers sync instantly')),
      el('div', null, el('strong', null, 'FAST'), el('span', null, 'Speed earns points')),
      el('div', null, el('strong', null, 'SIMPLE'), el('span', null, 'No sign-up required'))));

    setTimeout(() => {
      const roomInp = $('#room-input');
      const nameInp = $('#nickname-input');
      if (urlRoom && nameInp) nameInp.focus(); else if (roomInp) roomInp.focus();
      if (nameInp) nameInp.addEventListener('keydown', (e) => { if (e.key === 'Enter') handleJoin(); });
      if (roomInp) roomInp.addEventListener('keydown', (e) => { if (e.key === 'Enter') nameInp?.focus(); });
    }, 0);
  }

  async function handleJoin() {
    if (joinInFlight) return;
    const roomInp = $('#room-input');
    const nameInp = $('#nickname-input');
    const errorEl = $('[data-testid="join-error"]');
    const btn = $('[data-testid="join-btn"]');
    const code = roomInp.value.trim().toUpperCase();
    const nickname = nameInp.value.trim();

    if (!code) { errorEl.textContent = 'Enter a room code to continue.'; roomInp.focus(); return; }
    if (!nickname) { errorEl.textContent = 'Add a display name to join.'; nameInp.focus(); return; }

    joinInFlight = true;
    btn.disabled = true;
    btn.innerHTML = 'Joining…';
    errorEl.textContent = '';
    try {
      const data = await api('/api/join', { method: 'POST', body: JSON.stringify({ nickname, room_code: code }) });
      playerId = data.player_id;
      playerNickname = nickname;
      roomCode = code;
      gameStatus = 'lobby';
      connectWS();
      render();
    } catch (err) {
      errorEl.textContent = err.message || 'Could not join this room.';
      btn.disabled = false;
      btn.innerHTML = 'Join room <span class="btn-arrow" aria-hidden="true">→</span>';
    } finally { joinInFlight = false; }
  }

  function renderLobby(app) {
    const card = el('section', { className: 'card lobby-screen', 'data-testid': 'lobby-screen' },
      el('div', { className: 'lobby-hero' },
        el('div', { className: 'eyebrow' }, 'YOU’RE IN'),
        el('h2', null, 'The room is warming up'),
        el('p', { className: 'subtitle' }, 'Keep this screen open. The host will start the first round when everyone is ready.')),
      el('div', { className: 'lobby-code' }, el('span', null, 'ROOM'), el('strong', null, roomCode)),
      el('div', { className: 'players-heading' }, el('h3', null, 'Players in the room'), el('span', { className: 'badge', 'data-testid': 'player-count-badge' }, '0')),
      el('ul', { className: 'player-list', 'data-testid': 'player-list', 'aria-live': 'polite' }),
      el('div', { className: 'waiting-message' }, el('div', { className: 'pulse-ring', 'aria-hidden': 'true' }), el('span', null, 'Waiting for the host to start…'))
    );
    app.appendChild(card);
    api('/api/players').then(updatePlayerList).catch(() => {});
  }

  function updatePlayerList(players) {
    const list = $('[data-testid="player-list"]');
    const badge = $('[data-testid="player-count-badge"]');
    if (!list) return;
    list.innerHTML = '';
    const safePlayers = players || [];
    safePlayers.forEach((p) => {
      const isYou = p.player_id === playerId;
      list.appendChild(el('li', { className: `player-chip${isYou ? ' you' : ''}`, 'data-testid': 'player-chip' },
        el('span', { className: 'avatar-dot', 'aria-hidden': 'true' }, (p.nickname || '?').charAt(0).toUpperCase()),
        el('span', null, p.nickname), isYou ? el('span', { className: 'you-tag' }, 'YOU') : null));
    });
    if (badge) badge.textContent = String(safePlayers.length);
  }

  function renderCountdown(app) {
    app.appendChild(el('section', { className: 'card countdown-screen', 'data-testid': 'countdown-screen' },
      el('div', { className: 'eyebrow' }, 'NEXT UP'),
      el('h2', null, 'Get ready to play'),
      el('p', { className: 'subtitle' }, `${totalQuestions} questions · fastest correct answers score more`),
      el('div', { className: 'countdown-circle', 'aria-live': 'assertive' },
        el('span', { className: 'countdown-number', 'data-testid': 'countdown-number' }, String(Math.max(0, countdownLeft)))),
      el('p', { className: 'countdown-note' }, 'Eyes on the question. Fingers ready.')));
  }

  function renderQuestion(app) {
    const q = currentQuestion;
    if (!q) { app.appendChild(el('section', { className: 'card waiting' }, el('div', { className: 'spinner' }), el('p', null, 'Loading the next question…'))); return; }
    const optLabels = ['A', 'B', 'C', 'D', 'E', 'F'];
    const isReveal = gameStatus === 'reveal';
    const progress = totalQuestions ? ((questionIndex + 1) / totalQuestions) * 100 : 0;
    const timerTone = timeLeft <= 2 ? 'critical' : timeLeft <= 5 ? 'warning' : '';

    const card = el('section', { className: 'card question-screen', 'data-testid': 'question-screen' },
      el('div', { className: 'question-topline' },
        el('div', null,
          el('div', { className: 'eyebrow' }, 'LIVE QUESTION'),
          el('span', { className: 'question-counter', 'data-testid': 'question-counter' }, `${questionIndex + 1} / ${totalQuestions}`)),
        el('div', { className: `timer-number ${timerTone}`, 'data-testid': 'timer-number', 'aria-label': isReveal ? 'Time is up' : `${timeLeft} seconds remaining` }, isReveal ? 'Time’s up' : `${timeLeft}s`)),
      el('div', { className: 'progress-track', 'aria-hidden': 'true' }, el('div', { className: 'progress-fill', style: `width:${progress}%` })),
      el('div', { className: 'timer-bar', role: 'progressbar', 'aria-label': 'Time remaining', 'aria-valuemin': '0', 'aria-valuemax': String(timeLimit), 'aria-valuenow': String(Math.max(0, timeLeft)) },
        el('div', { className: `timer-fill ${isReveal ? 'critical' : timerTone}`, id: 'timer-fill', 'data-testid': 'timer-fill', style: isReveal ? 'width:0%' : `width:${Math.max(0, (timeLeft / timeLimit) * 100)}%` })),
      el('h2', { className: 'question-text', 'data-testid': 'question-text' }, q.text),
      el('div', { className: 'options-grid', 'data-testid': 'options-grid', role: 'group', 'aria-label': 'Answer choices' },
        ...q.options.map((opt, i) => {
          let cls = 'option-btn';
          if (selectedAnswer === i) cls += ' selected';
          if (isReveal) {
            if (i === correctAnswer) cls += ' correct';
            if (selectedAnswer === i && i !== correctAnswer) cls += ' wrong';
            cls += ' disabled';
          }
          if (answerSubmitted && !isReveal) cls += ' disabled';
          return el('button', { className: cls, 'data-testid': `option-${i}`, onclick: () => handleAnswer(i), disabled: answerSubmitted || isReveal, 'aria-pressed': selectedAnswer === i ? 'true' : 'false' },
            el('span', { className: 'option-label', 'aria-hidden': 'true' }, optLabels[i] || String(i)),
            el('span', null, opt));
        })),
    );

    if (answerSubmitted && !isReveal) card.appendChild(el('div', { className: 'answer-locked', 'data-testid': 'answer-locked', role: 'status' }, el('span', { className: 'lock-icon', 'aria-hidden': 'true' }, '✓'), 'Answer locked. Stay sharp for the reveal.'));
    if (isReveal && myResult) card.appendChild(el('div', { className: `result-toast ${myResult.correct ? 'correct' : 'wrong'}`, 'data-testid': 'result-toast', role: 'status' },
      el('span', null, myResult.correct ? 'Correct answer' : (selectedAnswer == null ? 'No answer this round' : 'Not this time')),
      el('strong', { className: 'result-score' }, myResult.correct ? `+${myResult.score_earned || 0}` : '+0')));
    else if (isReveal && !myResult && selectedAnswer == null) card.appendChild(el('div', { className: 'result-toast wrong', 'data-testid': 'result-toast', role: 'status' }, 'Time’s up — no answer submitted'));
    if (isReveal) card.appendChild(el('div', { className: 'waiting-next', 'data-testid': 'waiting-next' }, el('div', { className: 'spinner small' }), el('p', null, 'The host will send the next question soon…')));
    app.appendChild(card);
  }

  async function handleAnswer(index) {
    if (answerSubmitted || gameStatus !== 'question') return;
    selectedAnswer = index;
    answerSubmitted = true;
    render();
    try {
      await api('/api/answer', { method: 'POST', body: JSON.stringify({ player_id: playerId, question_id: currentQuestion.id, answer: index }) });
    } catch (err) {
      if (err.message && err.message.includes('player not found')) { resetAll(); render(); notify('Your session expired. Join the room again.', 'error'); }
      else notify('Answer sent locally; waiting for the reveal.', 'neutral');
    }
  }

  function renderFinished(app) {
    app.appendChild(el('section', { className: 'card finished-screen', 'data-testid': 'finished-screen' },
      el('div', { className: 'finish-burst', 'aria-hidden': 'true' }, '★'),
      el('div', { className: 'eyebrow' }, 'FINAL RESULTS'),
      el('h2', null, 'Game over'),
      myRank
        ? el('div', { className: 'rank-display', 'data-testid': 'rank-display' },
            el('p', { className: 'rank-label' }, 'Your final rank'),
            el('div', { className: `rank-number${myRank <= 3 ? ' top' : ''}` }, `#${myRank}`),
            myResult ? el('p', { className: 'total-score' }, `${myResult.total_score || 0} total points`) : null)
        : el('div', { className: 'waiting' }, el('div', { className: 'spinner' }), el('p', null, 'Loading your final score…')),
      el('p', { className: 'finish-note' }, 'Nice run. Ready for another round?'),
      el('button', { className: 'btn btn-secondary', 'data-testid': 'play-again-btn', onclick: () => { resetAll(); render(); } }, 'Join another room')));
  }

  document.addEventListener('DOMContentLoaded', render);
})();
