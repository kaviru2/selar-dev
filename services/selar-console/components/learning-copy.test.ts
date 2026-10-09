import {readFileSync} from 'node:fs';
import {expect,it} from 'vitest';
it('current learning UI avoids approval workflow language',()=>{
 for(const file of ['components/Topbar.tsx','app/(app)/onboarding/page.tsx','app/(app)/chat/page.tsx']){
  const text=readFileSync(file,'utf8');
  expect(text).not.toMatch(/Y to confirm|confirm, reject, or relabel|before accepting a connection/);
 }
});
