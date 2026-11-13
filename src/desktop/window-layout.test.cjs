const {test}=require('node:test'),assert=require('node:assert/strict');
const {popupBounds}=require('./window-layout.cjs');
test('compact popup stays within each screen work area',()=>{
 for(const work of [{x:0,y:0,width:1920,height:1040},{x:-1280,y:0,width:1280,height:720},{x:0,y:0,width:300,height:300}])for(const tray of [null,{x:work.x+work.width-10,y:work.y+work.height,width:20,height:20},{x:work.x,y:work.y,width:20,height:20}])for(const prefs of [true,false]){
  const b=popupBounds(work,tray,prefs);assert.ok(b.x>=work.x&&b.y>=work.y);assert.ok(b.x+b.width<=work.x+work.width&&b.y+b.height<=work.y+work.height);assert.ok(b.width<=340);assert.ok(b.height<=(prefs?560:360));
 }
});
