import{c as n}from"./index-D0K5ER8T.js";/**
 * @license @lucide/vue v1.17.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */const r=[["rect",{width:"14",height:"14",x:"8",y:"8",rx:"2",ry:"2",key:"17jyea"}],["path",{d:"M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2",key:"zix9uf"}]],d=n("copy",r);async function s(c){var o;const t=c??"";if(t==="")return!1;if(typeof navigator<"u"&&((o=navigator.clipboard)!=null&&o.writeText)&&window.isSecureContext)try{return await navigator.clipboard.writeText(t),!0}catch{}try{const e=document.createElement("textarea");e.value=t,e.setAttribute("readonly",""),e.style.position="fixed",e.style.top="0",e.style.left="0",e.style.width="1px",e.style.height="1px",e.style.opacity="0",document.body.appendChild(e),e.focus(),e.select(),e.setSelectionRange(0,t.length);const a=document.execCommand("copy");return document.body.removeChild(e),a}catch{return!1}}export{d as C,s as c};
