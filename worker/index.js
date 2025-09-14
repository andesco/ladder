// Import the Go WASM runtime and module
import './public/wasm_exec.js';
import wasmModule from './main.wasm';

let wasmInstance = null;
let isInitialized = false;
let initPromise = null;

async function initializeWasm() {
  if (initPromise) {
    return initPromise;
  }
  
  initPromise = (async () => {
    let retryCount = 0;
    const maxRetries = 3;
    
    while (retryCount < maxRetries) {
      try {
        console.log(`Starting WASM initialization... (attempt ${retryCount + 1}/${maxRetries})`);
        
        // Create a new Go runtime instance
        const go = new Go();
        
        console.log('Creating WASM instance...');
        console.log('wasmModule type:', typeof wasmModule);
        console.log('wasmModule constructor:', wasmModule.constructor.name);
        
        // Validate WASM module before instantiation
        if (!wasmModule || typeof wasmModule !== 'object') {
          throw new Error('Invalid WASM module: expected WebAssembly.Module');
        }
        
        // In Workers, the import gives us a WebAssembly.Module directly
        // We need to instantiate it manually
        wasmInstance = new WebAssembly.Instance(wasmModule, go.importObject);
        console.log('WASM instance created:', wasmInstance);
        
        console.log('WASM instantiated, starting Go program...');
        
        // Start the Go program - this expects a WebAssembly.Instance
        const runPromise = go.run(wasmInstance);
        console.log('Go.run() called successfully');
        
        // Give the Go program a moment to start
        await new Promise(resolve => setTimeout(resolve, 1000));
        
        console.log('Go program should be running, waiting for exports...');
        
        // Wait for the Go fetch function to be available with exponential backoff
        let retries = 50; // 5 seconds total
        const baseDelay = 100;
        while (retries > 0 && typeof globalThis.goFetch !== 'function') {
          await new Promise(resolve => setTimeout(resolve, baseDelay));
          retries--;
        }
        
        if (typeof globalThis.goFetch === 'function') {
          console.log('Go fetch function is available');
          isInitialized = true;
          return true;
        } else {
          throw new Error('Go fetch function not available after waiting');
        }
      } catch (error) {
        retryCount++;
        console.error(`WASM initialization failed (attempt ${retryCount}/${maxRetries}):`, error);
        
        if (retryCount >= maxRetries) {
          console.error('Max retries exceeded, WASM initialization failed permanently');
          throw new Error(`WASM initialization failed after ${maxRetries} attempts: ${error.message}`);
        }
        
        // Wait before retrying with exponential backoff
        const retryDelay = 1000 * Math.pow(2, retryCount - 1);
        console.log(`Retrying in ${retryDelay}ms...`);
        await new Promise(resolve => setTimeout(resolve, retryDelay));
        
        // Reset state for retry
        wasmInstance = null;
        isInitialized = false;
      }
    }
  })();
  
  return initPromise;
}

export default {
  async fetch(request, env, ctx) {
    const url = new URL(request.url);
    const path = url.pathname;
    const startTime = Date.now();
    
    try {
      // Initialize WASM module if not already done
      if (!isInitialized) {
        console.log('Initializing WASM for request to:', path);
        const success = await initializeWasm();
        if (!success) {
          return new Response('Failed to initialize Go WASM runtime. Please try again in a few moments.', { 
            status: 503,
            headers: { 
              'Content-Type': 'text/plain',
              'Retry-After': '5'
            }
          });
        }
      }

      // Validate Go fetch function is available
      if (typeof globalThis.goFetch !== 'function') {
        console.error('Go fetch function not available after initialization');
        return new Response('Go WASM runtime not properly initialized. Please refresh and try again.', { 
          status: 503,
          headers: { 
            'Content-Type': 'text/plain',
            'Retry-After': '5'
          }
        });
      }

      // Call the Go-exported fetch function with timeout protection
      console.log('Calling Go fetch function for path:', path);
      const timeoutPromise = new Promise((_, reject) =>
        setTimeout(() => reject(new Error('Go fetch timeout')), 30000)
      );
      
      const goFetchPromise = globalThis.goFetch(request, env, ctx);
      const result = await Promise.race([goFetchPromise, timeoutPromise]);
      
      const duration = Date.now() - startTime;
      console.log(`Request completed in ${duration}ms`);
      
      return result;
      
    } catch (error) {
      const duration = Date.now() - startTime;
      console.error(`Worker error after ${duration}ms:`, error);
      
      // Return user-friendly error messages based on error type
      if (error.message.includes('timeout')) {
        return new Response('Request timeout. The server took too long to respond.', { 
          status: 504,
          headers: { 'Content-Type': 'text/plain' }
        });
      } else if (error.message.includes('WASM')) {
        return new Response('Service temporarily unavailable. Please try again.', { 
          status: 503,
          headers: { 
            'Content-Type': 'text/plain',
            'Retry-After': '10'
          }
        });
      } else {
        return new Response('An unexpected error occurred. Please try again.', { 
          status: 500,
          headers: { 'Content-Type': 'text/plain' }
        });
      }
    }
  },
};
