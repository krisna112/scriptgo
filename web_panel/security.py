import time
from functools import wraps
from flask import request, Response, abort

# --- CONFIGURATION ---
RATE_LIMIT_LOGIN = 5  # attempts
RATE_LIMIT_WINDOW = 60  # seconds
login_attempts = {}

# --- MIDDLEWARE ---

def add_security_headers(response):
    """Adds security headers to every response"""
    response.headers['X-Content-Type-Options'] = 'nosniff'
    response.headers['X-Frame-Options'] = 'SAMEORIGIN'
    response.headers['X-XSS-Protection'] = '1; mode=block'
    response.headers['Referrer-Policy'] = 'strict-origin-when-cross-origin'
    response.headers['Content-Security-Policy'] = "default-src 'self' https: 'unsafe-inline' 'unsafe-eval'; img-src 'self' data: https:;"
    return response

# --- RATE LIMITING ---

def rate_limit_check():
    """Checks rate limits for login"""
    ip = request.remote_addr
    now = time.time()
    
    # Cleanup old entries
    if ip in login_attempts:
        attempts, start_time = login_attempts[ip]
        if now - start_time > RATE_LIMIT_WINDOW:
            del login_attempts[ip]
    
    if request.path == '/login' and request.method == 'POST':
        if ip not in login_attempts:
            login_attempts[ip] = [1, now]
        else:
            attempts, start_time = login_attempts[ip]
            if attempts >= RATE_LIMIT_LOGIN:
                abort(429, description="Too many login attempts. Try again later.")
            login_attempts[ip] = [attempts + 1, start_time]

# --- WAF (Web Application Firewall) ---

COMMON_ATTACK_PATTERNS = [
    "<script>", "javascript:", "vbscript:", 
    "onload=", "onerror=", " union select ", 
    "/etc/passwd", "../..", "; cat "
]

def waf_check():
    """Basic WAF to block common attack vectors in inputs"""
    # Check Args
    for key, value in request.args.items():
        if any(pat in str(value).lower() for pat in COMMON_ATTACK_PATTERNS):
            abort(403, description="Malicious detected.")
            
    # Check Form Data
    if request.method == 'POST':
        for key, value in request.form.items():
            if any(pat in str(value).lower() for pat in COMMON_ATTACK_PATTERNS):
                abort(403, description="Malicious Input detected.")
