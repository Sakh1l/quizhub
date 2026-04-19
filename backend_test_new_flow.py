#!/usr/bin/env python3
"""
QuizHub Backend API Testing Suite - NEW GAME FLOW
Tests the new game flow requirements:
1. POST /api/game/start returns status 'countdown' (not 'question' immediately)
2. After 10 seconds, game auto-transitions to first question (server-side timer)
3. POST /api/answer records answer and returns {recorded: true} (NOT correct/wrong)
4. POST /api/game/next only works in 'reveal' state, returns 400 otherwise
5. GET /api/game/state returns correct_answer only in 'reveal' state
"""

import requests
import json
import sys
import time
from datetime import datetime

class QuizHubNewFlowTester:
    def __init__(self, base_url="https://56ba0c32-f66d-46a8-aaf5-8fc5eb1d5b03.preview.emergentagent.com"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({'Content-Type': 'application/json'})
        self.tests_run = 0
        self.tests_passed = 0
        self.player_id = None
        self.player_nickname = None
        self.current_question_id = None
        self.admin_token = None
        
    def log(self, message, level="INFO"):
        timestamp = datetime.now().strftime("%H:%M:%S")
        print(f"[{timestamp}] {level}: {message}")
        
    def run_test(self, name, method, endpoint, expected_status, data=None, description=""):
        """Run a single API test"""
        url = f"{self.base_url}/api/{endpoint}"
        self.tests_run += 1
        
        self.log(f"🔍 Testing {name} - {description}")
        
        try:
            if method == 'GET':
                response = self.session.get(url)
            elif method == 'POST':
                response = self.session.post(url, json=data)
                
            success = response.status_code == expected_status
            
            if success:
                self.tests_passed += 1
                self.log(f"✅ PASSED - Status: {response.status_code}", "SUCCESS")
                try:
                    response_data = response.json()
                    self.log(f"   Response: {json.dumps(response_data, indent=2)[:300]}...")
                    return True, response_data
                except:
                    return True, {}
            else:
                self.log(f"❌ FAILED - Expected {expected_status}, got {response.status_code}", "ERROR")
                try:
                    error_data = response.json()
                    self.log(f"   Error: {error_data}")
                except:
                    self.log(f"   Raw response: {response.text[:200]}")
                return False, {}
                
        except Exception as e:
            self.log(f"❌ FAILED - Exception: {str(e)}", "ERROR")
            return False, {}
    
    def test_health_endpoint(self):
        """Test GET /api/health"""
        success, data = self.run_test(
            "Health Check", 
            "GET", 
            "health", 
            200,
            description="Should return status ok"
        )
        
        if success and data.get('status') == 'ok':
            self.log("✅ Health endpoint working correctly")
            return True
        return False
    
    def test_join_player(self):
        """Test POST /api/join creates player"""
        test_nickname = f"TestPlayer_{int(time.time())}"
        success, data = self.run_test(
            "Join Game",
            "POST",
            "join",
            201,
            data={"nickname": test_nickname},
            description="Should create player"
        )
        
        if success and 'player_id' in data:
            self.player_id = data['player_id']
            self.player_nickname = test_nickname
            self.log(f"✅ Player created: {self.player_id}")
            return True
        return False
    
    def test_admin_auth(self):
        """Test POST /api/admin/auth with PIN 1234"""
        success, data = self.run_test(
            "Admin Auth",
            "POST",
            "admin/auth",
            200,
            data={"pin": "1234"},
            description="Should return token for PIN 1234"
        )
        
        if success and 'token' in data:
            self.admin_token = data['token']
            self.session.headers.update({'X-Admin-Token': self.admin_token})
            self.log(f"✅ Admin token obtained")
            return True
        return False
    
    def test_start_game_countdown(self):
        """Test POST /api/game/start returns status 'countdown' (not 'question' immediately)"""
        success, data = self.run_test(
            "Start Game (NEW FLOW)",
            "POST",
            "game/start",
            200,
            description="Should return status 'countdown' not 'question'"
        )
        
        if success:
            if data.get('status') != 'countdown':
                self.log(f"❌ Expected status 'countdown', got '{data.get('status')}'", "ERROR")
                return False
            if 'duration' not in data:
                self.log("❌ Missing 'duration' field in countdown response", "ERROR")
                return False
            if data.get('duration') != 10:
                self.log(f"❌ Expected duration 10, got {data.get('duration')}", "ERROR")
                return False
            
            self.log("✅ Game start returns countdown status correctly")
            return True
        return False
    
    def test_countdown_to_question_transition(self):
        """Test that after 10 seconds, game auto-transitions to first question"""
        self.log("⏳ Waiting for 10-second countdown to complete...")
        
        # Wait 11 seconds to ensure transition
        time.sleep(11)
        
        success, data = self.run_test(
            "Game State After Countdown",
            "GET",
            "game/state",
            200,
            description="Should show question state after countdown"
        )
        
        if success:
            if data.get('status') != 'question':
                self.log(f"❌ Expected status 'question' after countdown, got '{data.get('status')}'", "ERROR")
                return False
            if 'current_question' not in data:
                self.log("❌ Missing 'current_question' after countdown", "ERROR")
                return False
            
            # Store question ID for answer test
            self.current_question_id = data['current_question'].get('id')
            self.log(f"✅ Game transitioned to question state, question ID: {self.current_question_id}")
            return True
        return False
    
    def test_answer_returns_recorded_only(self):
        """Test POST /api/answer records answer and returns {recorded: true} (NOT correct/wrong)"""
        if not self.player_id or not self.current_question_id:
            self.log("❌ Missing player_id or question_id for answer test", "ERROR")
            return False
            
        success, data = self.run_test(
            "Submit Answer (NEW FLOW)",
            "POST",
            "answer",
            200,
            data={
                "player_id": self.player_id,
                "question_id": self.current_question_id,
                "answer": 0
            },
            description="Should return {recorded: true} only, NOT correct/wrong"
        )
        
        if success:
            if data.get('recorded') != True:
                self.log(f"❌ Expected 'recorded: true', got {data}", "ERROR")
                return False
            if 'correct' in data or 'correct_answer' in data:
                self.log(f"❌ Answer response should NOT reveal correct/wrong: {data}", "ERROR")
                return False
            
            self.log("✅ Answer submission returns recorded:true only (no reveal)")
            return True
        return False
    
    def test_next_question_fails_in_question_state(self):
        """Test POST /api/game/next only works in 'reveal' state, returns 400 otherwise"""
        success, data = self.run_test(
            "Next Question (Should Fail)",
            "POST",
            "game/next",
            400,
            description="Should return 400 when not in reveal state"
        )
        
        if success:
            self.log("✅ Next question correctly fails when not in reveal state")
            return True
        return False
    
    def test_game_state_no_correct_answer_in_question_mode(self):
        """Test GET /api/game/state does NOT return correct_answer in question state"""
        success, data = self.run_test(
            "Game State (Question Mode)",
            "GET",
            "game/state",
            200,
            description="Should NOT show correct_answer in question state"
        )
        
        if success:
            if 'correct_answer' in data:
                self.log(f"❌ Game state should NOT reveal correct_answer in question mode: {data}", "ERROR")
                return False
            
            self.log("✅ Game state correctly hides correct_answer in question mode")
            return True
        return False
    
    def test_wait_for_timer_and_reveal(self):
        """Wait for question timer to expire and check reveal state"""
        self.log("⏳ Waiting for question timer to expire (15 seconds)...")
        
        # Wait for timer to expire
        time.sleep(16)
        
        success, data = self.run_test(
            "Game State After Timer",
            "GET",
            "game/state",
            200,
            description="Should show reveal state after timer expires"
        )
        
        if success:
            if data.get('status') != 'reveal':
                self.log(f"❌ Expected status 'reveal' after timer, got '{data.get('status')}'", "ERROR")
                return False
            if 'correct_answer' not in data:
                self.log("❌ Missing 'correct_answer' in reveal state", "ERROR")
                return False
            
            self.log(f"✅ Game transitioned to reveal state with correct_answer: {data.get('correct_answer')}")
            return True
        return False
    
    def test_next_question_works_in_reveal_state(self):
        """Test POST /api/game/next works in reveal state"""
        success, data = self.run_test(
            "Next Question (Should Work)",
            "POST",
            "game/next",
            200,
            description="Should work in reveal state"
        )
        
        if success:
            self.log("✅ Next question works correctly in reveal state")
            return True
        return False
    
    def test_leaderboard(self):
        """Test GET /api/leaderboard returns sorted entries"""
        success, data = self.run_test(
            "Get Leaderboard",
            "GET",
            "leaderboard",
            200,
            description="Should return sorted entries"
        )
        
        if success and isinstance(data, list):
            self.log("✅ Leaderboard returns sorted entries")
            return True
        return False
    
    def test_reset_game(self):
        """Test POST /api/game/reset clears everything"""
        success, data = self.run_test(
            "Reset Game",
            "POST",
            "game/reset",
            200,
            description="Should clear everything"
        )
        
        if success and data.get('status') == 'reset':
            self.log("✅ Game reset works correctly")
            return True
        return False
    
    def run_new_flow_tests(self):
        """Run all NEW game flow tests in sequence"""
        self.log("🚀 Starting QuizHub NEW GAME FLOW Tests")
        self.log(f"   Base URL: {self.base_url}")
        
        test_methods = [
            self.test_health_endpoint,
            self.test_join_player,
            self.test_admin_auth,
            self.test_start_game_countdown,
            self.test_countdown_to_question_transition,
            self.test_answer_returns_recorded_only,
            self.test_next_question_fails_in_question_state,
            self.test_game_state_no_correct_answer_in_question_mode,
            self.test_wait_for_timer_and_reveal,
            self.test_next_question_works_in_reveal_state,
            self.test_leaderboard,
            self.test_reset_game
        ]
        
        for test_method in test_methods:
            try:
                if not test_method():
                    self.log(f"❌ Test {test_method.__name__} failed", "ERROR")
                time.sleep(1)  # Small delay between tests
            except Exception as e:
                self.log(f"❌ Test {test_method.__name__} failed with exception: {e}", "ERROR")
        
        # Print summary
        self.log("=" * 60)
        self.log(f"📊 NEW FLOW Test Results: {self.tests_passed}/{self.tests_run} passed")
        
        if self.tests_passed == self.tests_run:
            self.log("🎉 All NEW FLOW tests passed!", "SUCCESS")
            return 0
        else:
            self.log(f"❌ {self.tests_run - self.tests_passed} tests failed", "ERROR")
            return 1

def main():
    tester = QuizHubNewFlowTester()
    return tester.run_new_flow_tests()

if __name__ == "__main__":
    sys.exit(main())