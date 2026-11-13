import sys
from pathlib import Path
import unittest
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'tools/scripts'))
from merge_headscale_extra_records import merge_records

class MagicDNSMergeTest(unittest.TestCase):
    def test_merge_preserves_other_records(self):
        old = [{'name':'manual.example.org','type':'A','value':'192.0.2.1'},
               {'name':'old.laptop.intra.example.org','type':'A','value':'100.64.1.1'}]
        fresh = [{'name':'media.laptop.intra.example.org','type':'A','value':'100.64.1.2'}]
        result = merge_records(fresh,old,'laptop.intra.example.org')
        self.assertEqual(len(result),2)
        self.assertEqual({r['name'] for r in result}, {'manual.example.org','media.laptop.intra.example.org'})
    def test_reject_external_ip_and_domain(self):
        bads = [
            [{'name':'media.laptop.intra.example.org','type':'A','value':'8.8.8.8'}],
            [{'name':'media.other.example.org','type':'A','value':'100.64.2.4'}],
            [{'name':'media.laptop.intra.example.org','type':'CNAME','value':'100.64.2.4'}]
        ]
        for rows in bads:
            with self.subTest(rows=rows), self.assertRaises(ValueError):
                merge_records(rows,[], 'laptop.intra.example.org')

if __name__ == '__main__': unittest.main()
