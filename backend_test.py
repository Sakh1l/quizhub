#!/usr/bin/env python3
"""
QuizHub Backend API Testing Suite
Tests all API endpoints for the multiplayer trivia game
"""

import requests
import json
import sys
import time
from datetime import datetime

class QuizHubAPITester:
    def __init__(self, base_url="https://bug-hunter-192.preview.emergentagent.com"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session.headers.update({'Content-Type': 'application/json'})
        self.tests_run = 0
        self.tests_passed = 0
        self.player_id = None
        self.player_nickname = None
        self.current_question_id = None
        self.admin_token = None
        self.test_question_id = None
        
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
            elif method == 'PUT':
                response = self.session.put(url, json=data)
            elif method == 'DELETE':
                response = self.session.delete(url)
                
            success = response.status_code == expected_status
            
            if success:
                self.tests_passed += 1
                self.log(f"✅ PASSED - Status: {response.status_code}", "SUCCESS")
                try:
                    response_data = response.json()
                    self.log(f"   Response: {json.dumps(response_data, indent=2)[:200]}...")
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
            description="Should return status, version, and player_count"
        )
        
        if success:
            required_fields = ['status', 'version', 'player_count']
            for field in required_fields:
                if field not in data:
                    self.log(f"❌ Missing required field: {field}", "ERROR")
                    return False
            
            if data.get('status') != 'ok':
                self.log(f"❌ Expected status 'ok', got '{data.get('status')}'", "ERROR")
                return False
                
        return success
    
    def test_join_with_nickname(self):
        """Test POST /api/join with valid nickname"""
        test_nickname = f"TestPlayer_{int(time.time())}"
        success, data = self.run_test(
            "Join Game (Valid)",
            "POST",
            "join",
            201,
            data={"nickname": test_nickname},
            description="Should create player and return player_id"
        )
        
        if success:
            if 'player_id' not in data:
                self.log("❌ Missing player_id in response", "ERROR")
                return False
            if 'nickname' not in data:
                self.log("❌ Missing nickname in response", "ERROR")
                return False
            if data.get('score', -1) != 0:
                self.log("❌ Initial score should be 0", "ERROR")
                return False
                
            # Store for later tests
            self.player_id = data['player_id']
            self.player_nickname = test_nickname
            self.log(f"   Player created: {self.player_id}")
            
        return success
    
    def test_join_empty_nickname(self):
        """Test POST /api/join with empty nickname"""
        success, data = self.run_test(
            "Join Game (Empty Nickname)",
            "POST", 
            "join",
            400,
            data={"nickname": ""},
            description="Should return 400 error for empty nickname"
        )
        return success
    
    def test_get_players(self):
        """Test GET /api/players"""
        success, data = self.run_test(
            "Get Players",
            "GET",
            "players", 
            200,
            description="Should return array of players"
        )
        
        if success:
            if not isinstance(data, list):
                self.log("❌ Response should be an array", "ERROR")
                return False
            
            # Should contain our test player
            found_player = False
            for player in data:
                if player.get('player_id') == self.player_id:
                    found_player = True
                    break
            
            if not found_player:
                self.log("❌ Test player not found in players list", "ERROR")
                return False
                
        return success
    
    def test_start_game(self):
        """Test POST /api/game/start"""
        success, data = self.run_test(
            "Start Game",
            "POST",
            "game/start",
            200,
            description="Should start game and return question state"
        )
        
        if success:
            required_fields = ['status', 'current_question', 'question_index', 'total_questions', 'time_left']
            for field in required_fields:
                if field not in data:
                    self.log(f"❌ Missing required field: {field}", "ERROR")
                    return False
            
            if data.get('status') != 'question':
                self.log(f"❌ Expected status 'question', got '{data.get('status')}'", "ERROR")
                return False
                
            # Store current question for answer test
            if data.get('current_question'):
                self.current_question_id = data['current_question'].get('id')
                self.log(f"   Current question ID: {self.current_question_id}")
                
        return success
    
    def test_answer_valid(self):
        """Test POST /api/answer with valid data"""
        if not self.player_id or not self.current_question_id:
            self.log("❌ Missing player_id or question_id for answer test", "ERROR")
            return False
            
        success, data = self.run_test(
            "Submit Answer (Valid)",
            "POST",
            "answer",
            200,
            data={
                "player_id": self.player_id,
                "question_id": self.current_question_id,
                "answer": 0  # First option
            },
            description="Should process answer and return result"
        )
        
        if success:
            required_fields = ['correct', 'correct_answer', 'score_earned', 'total_score']
            for field in required_fields:
                if field not in data:
                    self.log(f"❌ Missing required field: {field}", "ERROR")
                    return False
                    
        return success
    
    def test_answer_duplicate(self):
        """Test POST /api/answer with duplicate answer"""
        if not self.player_id or not self.current_question_id:
            self.log("❌ Missing player_id or question_id for duplicate answer test", "ERROR")
            return False
            
        success, data = self.run_test(
            "Submit Answer (Duplicate)",
            "POST",
            "answer",
            409,
            data={
                "player_id": self.player_id,
                "question_id": self.current_question_id,
                "answer": 1  # Different option
            },
            description="Should return 409 conflict for duplicate answer"
        )
        return success
    
    def test_get_leaderboard(self):
        """Test GET /api/leaderboard"""
        success, data = self.run_test(
            "Get Leaderboard",
            "GET",
            "leaderboard",
            200,
            description="Should return sorted player entries"
        )
        
        if success:
            if not isinstance(data, list):
                self.log("❌ Response should be an array", "ERROR")
                return False
                
            # Check if entries have required fields
            for entry in data:
                required_fields = ['rank', 'player_id', 'nickname', 'score']
                for field in required_fields:
                    if field not in entry:
                        self.log(f"❌ Missing required field in leaderboard entry: {field}", "ERROR")
                        return False
                        
        return success
    
    def test_next_question(self):
        """Test POST /api/game/next"""
        success, data = self.run_test(
            "Next Question",
            "POST",
            "game/next",
            200,
            description="Should advance to next question or finish game"
        )
        
        if success:
            # Could be next question or finished state
            if data.get('status') == 'question':
                required_fields = ['current_question', 'question_index', 'total_questions']
                for field in required_fields:
                    if field not in data:
                        self.log(f"❌ Missing required field: {field}", "ERROR")
                        return False
            elif data.get('status') == 'finished':
                self.log("   Game finished - no more questions")
            else:
                self.log(f"❌ Unexpected status: {data.get('status')}", "ERROR")
                return False
                
        return success
    
    def test_reset_game(self):
        """Test POST /api/game/reset"""
        success, data = self.run_test(
            "Reset Game",
            "POST",
            "game/reset",
            200,
            description="Should reset game to lobby state"
        )
        
        if success:
            if data.get('status') != 'reset':
                self.log(f"❌ Expected status 'reset', got '{data.get('status')}'", "ERROR")
                return False
                
        return success
    
    def test_admin_auth_correct_pin(self):
        """Test POST /api/admin/auth with correct PIN"""
        success, data = self.run_test(
            "Admin Auth (Correct PIN)",
            "POST",
            "admin/auth",
            200,
            data={"pin": "1234"},
            description="Should return admin token for correct PIN"
        )
        
        if success:
            if 'token' not in data:
                self.log("❌ Missing token in admin auth response", "ERROR")
                return False
            
            # Store admin token for protected endpoint tests
            self.admin_token = data['token']
            self.session.headers.update({'X-Admin-Token': self.admin_token})
            self.log(f"   Admin token obtained: {self.admin_token[:20]}...")
            
        return success
    
    def test_admin_auth_wrong_pin(self):
        """Test POST /api/admin/auth with wrong PIN"""
        # Temporarily remove admin token for this test
        old_token = self.session.headers.pop('X-Admin-Token', None)
        
        success, data = self.run_test(
            "Admin Auth (Wrong PIN)",
            "POST",
            "admin/auth",
            401,
            data={"pin": "wrong"},
            description="Should return 401 for wrong PIN"
        )
        
        # Restore admin token
        if old_token:
            self.session.headers['X-Admin-Token'] = old_token
            
        return success
    
    def test_admin_protected_without_token(self):
        """Test admin-protected endpoint without token"""
        # Temporarily remove admin token
        old_token = self.session.headers.pop('X-Admin-Token', None)
        
        success, data = self.run_test(
            "Admin Protected (No Token)",
            "POST",
            "admin/timer",
            401,
            data={"time_limit": 20},
            description="Should return 401 without admin token"
        )
        
        # Restore admin token
        if old_token:
            self.session.headers['X-Admin-Token'] = old_token
            
        return success
    
    def test_admin_set_timer(self):
        """Test POST /api/admin/timer with valid token"""
        success, data = self.run_test(
            "Admin Set Timer",
            "POST",
            "admin/timer",
            200,
            data={"time_limit": 20},
            description="Should set timer with valid admin token"
        )
        
        if success:
            if data.get('time_limit') != 20:
                self.log(f"❌ Expected time_limit 20, got {data.get('time_limit')}", "ERROR")
                return False
                
        return success
    
    def test_get_categories(self):
        """Test GET /api/categories"""
        success, data = self.run_test(
            "Get Categories",
            "GET",
            "categories",
            200,
            description="Should return list of question categories"
        )
        
        if success:
            if not isinstance(data, list):
                self.log("❌ Categories response should be an array", "ERROR")
                return False
                
        return success
    
    def test_add_question(self):
        """Test POST /api/questions/add with valid token"""
        test_question = {
            "text": "Test question for API testing?",
            "options": ["Option A", "Option B", "Option C", "Option D"],
            "answer": 1,
            "category": "test"
        }
        
        success, data = self.run_test(
            "Add Question",
            "POST",
            "questions/add",
            201,
            data=test_question,
            description="Should add question with valid admin token"
        )
        
        if success:
            if 'id' not in data:
                self.log("❌ Missing question id in response", "ERROR")
                return False
            
            # Store question ID for delete test
            self.test_question_id = data['id']
            self.log(f"   Question created with ID: {self.test_question_id}")
            
        return success
    
    def test_delete_question(self):
        """Test POST /api/questions/delete with valid token"""
        if not hasattr(self, 'test_question_id'):
            self.log("❌ No test question ID available for delete test", "ERROR")
            return False
            
        success, data = self.run_test(
            "Delete Question",
            "POST",
            "questions/delete",
            200,
            data={"id": self.test_question_id},
            description="Should delete question with valid admin token"
        )
        
        if success:
            if data.get('status') != 'deleted':
                self.log(f"❌ Expected status 'deleted', got '{data.get('status')}'", "ERROR")
                return False
                
        return success
    
    def test_websocket_endpoint(self):
        """Test WebSocket endpoint accessibility"""
        import websocket
        import ssl
        
        try:
            # Test WebSocket connection
            ws_url = self.base_url.replace('https://', 'wss://').replace('http://', 'ws://') + '/api/ws?role=player'
            
            self.log("🔍 Testing WebSocket Connection")
            self.log(f"   WebSocket URL: {ws_url}")
            
            # Create WebSocket connection with SSL context for wss://
            ws = websocket.create_connection(
                ws_url,
                timeout=10,
                sslopt={"cert_reqs": ssl.CERT_NONE} if ws_url.startswith('wss://') else None
            )
            
            # Send a test message (though the Go server might not respond to arbitrary messages)
            ws.send('{"test": "connection"}')
            
            # Close connection
            ws.close()
            
            self.tests_run += 1
            self.tests_passed += 1
            self.log("✅ PASSED - WebSocket connection successful", "SUCCESS")
            return True
            
        except Exception as e:
            self.tests_run += 1
            self.log(f"❌ FAILED - WebSocket connection failed: {str(e)}", "ERROR")
            return False
    
    def run_all_tests(self):
        """Run all API tests in sequence"""
        self.log("🚀 Starting QuizHub API Tests")
        self.log(f"   Base URL: {self.base_url}")
        
        test_methods = [
            self.test_health_endpoint,
            self.test_join_with_nickname,
            self.test_join_empty_nickname,
            self.test_get_players,
            self.test_start_game,
            self.test_answer_valid,
            self.test_answer_duplicate,
            self.test_get_leaderboard,
            self.test_next_question,
            self.test_reset_game,
            # Admin functionality tests
            self.test_admin_auth_correct_pin,
            self.test_admin_auth_wrong_pin,
            self.test_admin_protected_without_token,
            self.test_admin_set_timer,
            self.test_get_categories,
            self.test_add_question,
            self.test_delete_question,
            self.test_websocket_endpoint
        ]
        
        for test_method in test_methods:
            try:
                test_method()
                time.sleep(0.5)  # Small delay between tests
            except Exception as e:
                self.log(f"❌ Test {test_method.__name__} failed with exception: {e}", "ERROR")
        
        # Print summary
        self.log("=" * 50)
        self.log(f"📊 Test Results: {self.tests_passed}/{self.tests_run} passed")
        
        if self.tests_passed == self.tests_run:
            self.log("🎉 All tests passed!", "SUCCESS")
            return 0
        else:
            self.log(f"❌ {self.tests_run - self.tests_passed} tests failed", "ERROR")
            return 1

def main():
    tester = QuizHubAPITester()
    return tester.run_all_tests()

if __name__ == "__main__":
    sys.exit(main())