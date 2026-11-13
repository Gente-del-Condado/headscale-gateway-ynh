package org.headscalegateway.android;

import android.content.Context;
import android.net.*;
import java.math.BigInteger;
import java.net.InetAddress;
import java.util.*;

/** Rutas completas con exclusiones de LAN; funciona también antes de API 33. */
final class VpnRoutes {
    static final class Prefix {
        final BigInteger network; final int length,bits;
        Prefix(byte[] bytes,int length) {
            this.bits=bytes.length*8;this.length=length;
            this.network=new BigInteger(1,bytes).shiftRight(bits-length).shiftLeft(bits-length);
        }
        Prefix(BigInteger network,int length,int bits) {this.network=network;this.length=length;this.bits=bits;}
        boolean contains(Prefix other) {return bits==other.bits && length<=other.length && network.shiftRight(bits-length).equals(other.network.shiftRight(bits-length));}
        String address() throws Exception {
            byte[] raw=network.toByteArray(),bytes=new byte[bits/8];int n=Math.min(raw.length,bytes.length);
            System.arraycopy(raw,raw.length-n,bytes,bytes.length-n,n);return InetAddress.getByAddress(bytes).getHostAddress();
        }
    }
    static void remove(Prefix route,Prefix excluded,List<Prefix> result) {
        if(excluded.contains(route)) return;
        if(!route.contains(excluded)) {result.add(route);return;}
        int child=route.length+1;
        Prefix left=new Prefix(route.network,child,route.bits);
        Prefix right=new Prefix(route.network.setBit(route.bits-child),child,route.bits);
        remove(left,excluded,result);remove(right,excluded,result);
    }
    static void apply(Context context,VpnService.Builder builder,boolean allowLan) throws Exception {
        List<Prefix> routes=new ArrayList<>();routes.add(new Prefix(new byte[4],0));routes.add(new Prefix(new byte[16],0));
        List<Prefix> excluded=new ArrayList<>();
        for(String entry:new String[]{"127.0.0.0/8","::1/128","224.0.0.0/4","ff00::/8"}) {
            String[] parts=entry.split("/");excluded.add(new Prefix(InetAddress.getByName(parts[0]).getAddress(),Integer.parseInt(parts[1])));
        }
        if(allowLan) {
            ConnectivityManager cm=context.getSystemService(ConnectivityManager.class);
            for(Network network:cm.getAllNetworks()) {
                NetworkCapabilities capabilities=cm.getNetworkCapabilities(network);LinkProperties lp=cm.getLinkProperties(network);
                if(capabilities==null || lp==null || !capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN))continue;
                for(LinkAddress address:lp.getLinkAddresses()) excluded.add(new Prefix(address.getAddress().getAddress(),address.getPrefixLength()));
            }
        }
        for(Prefix exception:excluded) {List<Prefix> next=new ArrayList<>();for(Prefix route:routes)remove(route,exception,next);routes=next;}
        for(Prefix route:routes)builder.addRoute(route.address(),route.length);
    }
}
