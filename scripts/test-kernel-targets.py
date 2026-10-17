#!/usr/bin/env python3
"""Regression: the next-boot kernel is taken from presets, never uname alone."""
from pathlib import Path
import subprocess, tempfile, unittest
SCRIPT=Path(__file__).resolve().with_name('kernel-targets.sh')
@unittest.skipUnless(Path('/usr/lib/initcpio/functions').exists(), 'mkinitcpio is required')
class KernelTargets(unittest.TestCase):
 def setUp(self):
  self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup);self.path=Path(self.temp.name)
 def run_targets(self): return subprocess.run(['bash',str(SCRIPT),str(self.path)],capture_output=True,text=True)
 def test_running_and_next_image_are_both_checked(self):
  image=self.path/'next-kernel';data=bytearray(1024);data[0x20e:0x210]=(0x80).to_bytes(2,'little');version=b'7.99.12-next SMP\0';data[0x280:0x280+len(version)]=version;image.write_bytes(data)
  (self.path/'linux.preset').write_text(f'ALL_kver="{image}"\nPRESETS=(default fallback)\n')
  p=self.run_targets();self.assertEqual(p.returncode,0,p.stderr)
  versions=p.stdout.splitlines();self.assertEqual(len(versions),2);self.assertEqual(versions[1],'7.99.12-next')
 def test_per_preset_override_and_dedup(self):
  (self.path/'linux.preset').write_text('ALL_kver="7.99.12-next"\nPRESETS=(default fallback)\nfallback_kver="7.99.13-other"\n')
  p=self.run_targets();self.assertEqual(p.returncode,0,p.stderr);self.assertEqual(p.stdout.splitlines()[1:],['7.99.12-next','7.99.13-other'])
 def test_missing_boot_image_is_an_error(self):
  (self.path/'linux.preset').write_text('ALL_kver="/missing/boot/kernel"\nPRESETS=(default)\n')
  self.assertNotEqual(self.run_targets().returncode,0)
if __name__=='__main__':unittest.main()
