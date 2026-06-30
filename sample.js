// Sample minified production bundle for testing jspect
// Contains realistic patterns: endpoints, JWT, S3, GraphQL, comments, IPs

!function(window){const API_BASE="/api/v2",WS_URL="wss://realtime.example.com/ws",GRAPHQL_URL="/graphql";const config={apiBase:API_BASE,uploadEndpoint:"/api/v2/files/upload",authEndpoint:"/auth/login",refreshEndpoint:"/auth/refresh",debug:false};

// TODO: remove hardcoded fallback before prod deploy
const FALLBACK_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIn0.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c";

async function login(username,password){const res=await fetch("/api/v2/auth/login",{method:"POST",headers:{"Content-Type":"application/json","X-API-Key":"sk-prod-a8f3c2b1d4e5f6a7b8c9d0e1f2"},body:JSON.stringify({username:username,password:password})});return res.json()}

function getUser(id){return fetch(`/api/v2/users/${id}`)}

// FIXME: this should be behind auth middleware
function adminDashboard(){return fetch("/internal/admin/dashboard")}

function uploadFile(file){const formData=new FormData();formData.append("file",file);return fetch("/api/v2/files/upload",{method:"POST",body:formData})}

const s3Bucket="https://my-app-uploads.s3.us-east-1.amazonaws.com";
const cdnAsset="https://cdn.jsdelivr.net/npm/lodash@4.17.21/lodash.min.js";

function connectWebSocket(){const ws=new WebSocket(WS_URL);ws.onopen=()=>console.log("connected");ws.onmessage=e=>console.log("msg:",e.data);return ws}

const userQuery=`
  query GetUserProfile($id: ID!) {
    user(id: $id) {
      id
      name
      email
      roles
    }
  }
`;

const updateMutation=`
  mutation UpdateProfile($id: ID!, $input: ProfileInput!) {
    updateProfile(id: $id, input: $input) {
      success
      message
    }
  }
`;

// internal network config - do not expose to client builds
const METRICS_HOST="192.168.1.100";
const PRIVATE_DB_HOST="10.0.0.25";
const PUBLIC_CDN="104.16.85.20";

class ApiClient{constructor(baseUrl){this.baseUrl=baseUrl;this.token=null}setToken(token){this.token=token}async request(path,options={}){const headers={...options.headers,Authorization:`Bearer ${this.token}`};return fetch(`${this.baseUrl}${path}`,{...options,headers})}async get(path){return this.request(path,{method:"GET"})}async post(path,data){return this.request(path,{method:"POST",body:JSON.stringify(data)})}}

const client=new ApiClient(API_BASE);

// hardcoded test credentials - remove before merging to main
const TEST_PASSWORD="SuperSecret123!";

function isAdmin(user){return user&&user.role==="admin"}

window.AppConfig=config;
window.ApiClient=ApiClient;
}(window);
