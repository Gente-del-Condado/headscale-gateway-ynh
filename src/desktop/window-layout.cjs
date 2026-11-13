'use strict';
// Pure geometry, tested independently of compositor support for positioning.
function popupBounds(work,tray,preferences=false){
 const width=Math.min(340,work.width),height=Math.min(preferences?560:360,work.height);
 const anchor=tray&&tray.width>0&&tray.height>0;
 const x=anchor?Math.round(tray.x+tray.width/2-width/2):work.x+work.width-width-8;
 const y=anchor?(tray.y<work.y+work.height/2?tray.y+tray.height+6:tray.y-height-6):work.y+work.height-height-8;
 return {x:Math.max(work.x,Math.min(x,work.x+work.width-width)),y:Math.max(work.y,Math.min(y,work.y+work.height-height)),width,height};
}
module.exports={popupBounds};
